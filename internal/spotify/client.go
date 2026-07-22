package spotify

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"rhythmiq/internal/config"
	"rhythmiq/internal/models"
)

const (
	accountsBaseURL = "https://accounts.spotify.com"
	apiBaseURL      = "https://api.spotify.com/v1"
)

// OAuthScopes include everything needed for the listening dashboard.
var OAuthScopes = []string{
	"user-read-private",
	"user-top-read",
	"user-read-recently-played",
	"playlist-read-private",
	"user-library-read",
	"user-follow-read",
}

// Client wraps Spotify auth + API operations.
type Client struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	redirectURL  string
}

// NewClient initializes a spotify client from app config.
func NewClient(cfg config.Config) *Client {
	return &Client{
		httpClient:   &http.Client{Timeout: 20 * time.Second},
		clientID:     cfg.SpotifyClientID,
		clientSecret: cfg.SpotifyClientSecret,
		redirectURL:  cfg.SpotifyRedirectURL,
	}
}

// IsConfigured checks whether Spotify credentials are available.
func (c *Client) IsConfigured() bool {
	return c.clientID != "" && c.clientSecret != "" && c.redirectURL != ""
}

// AuthURL returns the user consent URL.
func (c *Client) AuthURL(state string) string {
	v := url.Values{}
	v.Set("client_id", c.clientID)
	v.Set("response_type", "code")
	v.Set("redirect_uri", c.redirectURL)
	v.Set("state", state)
	v.Set("scope", strings.Join(OAuthScopes, " "))
	v.Set("show_dialog", "true")
	return fmt.Sprintf("%s/authorize?%s", accountsBaseURL, v.Encode())
}

// ExchangeCode exchanges an auth code for access + refresh tokens.
func (c *Client) ExchangeCode(ctx context.Context, code string) (models.SpotifyToken, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", c.redirectURL)

	var payload tokenResponse
	if err := c.postTokenForm(ctx, form, &payload); err != nil {
		return models.SpotifyToken{}, err
	}

	return payload.toModel(), nil
}

// RefreshToken refreshes an expired access token.
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (models.SpotifyToken, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	var payload tokenResponse
	if err := c.postTokenForm(ctx, form, &payload); err != nil {
		return models.SpotifyToken{}, err
	}
	if payload.RefreshToken == "" {
		payload.RefreshToken = refreshToken
	}

	return payload.toModel(), nil
}

func (c *Client) postTokenForm(ctx context.Context, form url.Values, out any) error {
	if !c.IsConfigured() {
		return fmt.Errorf("spotify credentials are not configured")
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/api/token", accountsBaseURL),
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return fmt.Errorf("build spotify token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+basicAuth(c.clientID, c.clientSecret))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("spotify token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read spotify token response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("spotify token request returned %d: %s", resp.StatusCode, summarizeErrorBody(body))
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode spotify token response: %w", err)
	}
	return nil
}

// GetCurrentUser returns profile data from /me.
func (c *Client) GetCurrentUser(ctx context.Context, accessToken string) (models.UserProfile, error) {
	var payload struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Country     string `json:"country"`
		Product     string `json:"product"`
		Images      []struct {
			URL string `json:"url"`
		} `json:"images"`
	}
	if err := c.getJSON(ctx, accessToken, apiBaseURL+"/me", &payload); err != nil {
		return models.UserProfile{}, err
	}

	avatar := ""
	if len(payload.Images) > 0 {
		avatar = payload.Images[0].URL
	}
	return models.UserProfile{
		ID:          payload.ID,
		DisplayName: payload.DisplayName,
		Country:     payload.Country,
		Product:     payload.Product,
		AvatarURL:   avatar,
	}, nil
}

// trackObject mirrors the shape of a Spotify track across endpoints.
type trackObject struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Popularity int    `json:"popularity"`
	DurationMS int    `json:"duration_ms"`
	Album      struct {
		ID           string            `json:"id"`
		Name         string            `json:"name"`
		ReleaseDate  string            `json:"release_date"`
		Images       []imageObject     `json:"images"`
		ExternalURLs map[string]string `json:"external_urls"`
	} `json:"album"`
	Artists []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"artists"`
	ExternalURLs map[string]string `json:"external_urls"`
}

type imageObject struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func (t trackObject) toModel() models.TrackSummary {
	names := make([]string, 0, len(t.Artists))
	ids := make([]string, 0, len(t.Artists))
	for _, artist := range t.Artists {
		names = append(names, artist.Name)
		if artist.ID != "" {
			ids = append(ids, artist.ID)
		}
	}
	return models.TrackSummary{
		ID:            t.ID,
		Name:          t.Name,
		Artists:       names,
		ArtistIDs:     ids,
		Album:         t.Album.Name,
		AlbumID:       t.Album.ID,
		AlbumImageURL: pickImage(t.Album.Images),
		AlbumURL:      safeSpotifyExternalURL(t.Album.ExternalURLs["spotify"]),
		ReleaseYear:   parseReleaseYear(t.Album.ReleaseDate),
		DurationMS:    t.DurationMS,
		Popularity:    t.Popularity,
		ExternalURL:   safeSpotifyExternalURL(t.ExternalURLs["spotify"]),
	}
}

// parseReleaseYear reads the leading year from Spotify's release_date, which
// may be "2016", "2016-08", or "2016-08-20" depending on known precision.
func parseReleaseYear(raw string) int {
	if len(raw) < 4 {
		return 0
	}
	year, err := strconv.Atoi(raw[:4])
	if err != nil || year < 1900 || year > 2200 {
		return 0
	}
	return year
}

// artistObject mirrors the shape of a Spotify artist across endpoints.
type artistObject struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Genres    []string      `json:"genres"`
	Images    []imageObject `json:"images"`
	Followers struct {
		Total int `json:"total"`
	} `json:"followers"`
	Popularity   int               `json:"popularity"`
	ExternalURLs map[string]string `json:"external_urls"`
}

func (a artistObject) toModel() models.ArtistSummary {
	return models.ArtistSummary{
		ID:          a.ID,
		Name:        a.Name,
		Genres:      a.Genres,
		ImageURL:    pickImage(a.Images),
		Popularity:  a.Popularity,
		Followers:   a.Followers.Total,
		ExternalURL: safeSpotifyExternalURL(a.ExternalURLs["spotify"]),
	}
}

// pickImage returns the largest available image URL, which the editorial
// layout renders full-bleed.
func pickImage(images []imageObject) string {
	best := ""
	bestWidth := -1
	for _, image := range images {
		if image.URL == "" {
			continue
		}
		if image.Width > bestWidth {
			bestWidth = image.Width
			best = image.URL
		}
	}
	return best
}

// GetTopTracks fetches top tracks for a time range.
func (c *Client) GetTopTracks(ctx context.Context, accessToken, timeRange string, limit int) ([]models.TrackSummary, error) {
	endpoint := fmt.Sprintf("%s/me/top/tracks?time_range=%s&limit=%d", apiBaseURL, url.QueryEscape(timeRange), limit)
	var payload struct {
		Items []trackObject `json:"items"`
	}
	if err := c.getJSON(ctx, accessToken, endpoint, &payload); err != nil {
		return nil, err
	}

	tracks := make([]models.TrackSummary, 0, len(payload.Items))
	for _, item := range payload.Items {
		tracks = append(tracks, item.toModel())
	}
	return tracks, nil
}

// GetTopArtists fetches top artists for a time range.
func (c *Client) GetTopArtists(ctx context.Context, accessToken, timeRange string, limit int) ([]models.ArtistSummary, error) {
	endpoint := fmt.Sprintf("%s/me/top/artists?time_range=%s&limit=%d", apiBaseURL, url.QueryEscape(timeRange), limit)
	var payload struct {
		Items []artistObject `json:"items"`
	}
	if err := c.getJSON(ctx, accessToken, endpoint, &payload); err != nil {
		return nil, err
	}

	artists := make([]models.ArtistSummary, 0, len(payload.Items))
	for _, item := range payload.Items {
		artists = append(artists, item.toModel())
	}
	return artists, nil
}

// GetArtistsByID hydrates artists (genres + images) for IDs harvested from
// playback events, which only carry artist names and IDs.
func (c *Client) GetArtistsByID(ctx context.Context, accessToken string, ids []string) ([]models.ArtistSummary, error) {
	unique := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return nil, nil
	}

	const batchSize = 50
	artists := make([]models.ArtistSummary, 0, len(unique))
	for start := 0; start < len(unique); start += batchSize {
		end := start + batchSize
		if end > len(unique) {
			end = len(unique)
		}

		endpoint := fmt.Sprintf("%s/artists?ids=%s", apiBaseURL, url.QueryEscape(strings.Join(unique[start:end], ",")))
		var payload struct {
			Artists []artistObject `json:"artists"`
		}
		if err := c.getJSON(ctx, accessToken, endpoint, &payload); err != nil {
			return nil, err
		}
		for _, item := range payload.Artists {
			if item.ID == "" {
				continue
			}
			artists = append(artists, item.toModel())
		}
	}
	return artists, nil
}

// GetRecentlyPlayed fetches recently played tracks.
func (c *Client) GetRecentlyPlayed(ctx context.Context, accessToken string, limit int) ([]models.PlaybackEvent, error) {
	endpoint := fmt.Sprintf("%s/me/player/recently-played?limit=%d", apiBaseURL, limit)
	var payload struct {
		Items []struct {
			PlayedAt string      `json:"played_at"`
			Track    trackObject `json:"track"`
		} `json:"items"`
	}
	if err := c.getJSON(ctx, accessToken, endpoint, &payload); err != nil {
		return nil, err
	}

	result := make([]models.PlaybackEvent, 0, len(payload.Items))
	for _, item := range payload.Items {
		playedAt, err := time.Parse(time.RFC3339, item.PlayedAt)
		if err != nil {
			continue
		}
		result = append(result, models.PlaybackEvent{
			PlayedAt: playedAt,
			Track:    item.Track.toModel(),
		})
	}

	return result, nil
}

// GetSavedTrackCount fetches user's saved tracks count.
func (c *Client) GetSavedTrackCount(ctx context.Context, accessToken string) (int, error) {
	endpoint := fmt.Sprintf("%s/me/tracks?limit=1", apiBaseURL)
	var payload struct {
		Total int `json:"total"`
	}
	if err := c.getJSON(ctx, accessToken, endpoint, &payload); err != nil {
		return 0, err
	}
	return payload.Total, nil
}

// GetPlaylistCount fetches total number of playlists owned or followed.
func (c *Client) GetPlaylistCount(ctx context.Context, accessToken string) (int, error) {
	endpoint := fmt.Sprintf("%s/me/playlists?limit=1", apiBaseURL)
	var payload struct {
		Total int `json:"total"`
	}
	if err := c.getJSON(ctx, accessToken, endpoint, &payload); err != nil {
		return 0, err
	}
	return payload.Total, nil
}

// GetFollowingCount fetches followed artists count.
func (c *Client) GetFollowingCount(ctx context.Context, accessToken string) (int, error) {
	endpoint := fmt.Sprintf("%s/me/following?type=artist&limit=1", apiBaseURL)
	var payload struct {
		Artists struct {
			Total int `json:"total"`
		} `json:"artists"`
	}
	if err := c.getJSON(ctx, accessToken, endpoint, &payload); err != nil {
		return 0, err
	}
	return payload.Artists.Total, nil
}

func (c *Client) getJSON(ctx context.Context, accessToken, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build spotify request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("spotify api request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read spotify api response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("spotify api request returned %d: %s", resp.StatusCode, summarizeErrorBody(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode spotify response: %w", err)
	}
	return nil
}

func basicAuth(user, pass string) string {
	buf := bytes.NewBufferString(user)
	buf.WriteString(":")
	buf.WriteString(pass)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func summarizeErrorBody(body []byte) string {
	message := strings.TrimSpace(string(body))
	if message == "" {
		return "no error details"
	}
	if len(message) > 240 {
		message = message[:240] + "..."
	}
	return message
}

func safeSpotifyExternalURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return ""
	}

	host := strings.ToLower(parsed.Hostname())
	if host != "open.spotify.com" && host != "play.spotify.com" && host != "spotify.com" && host != "www.spotify.com" {
		return ""
	}
	return parsed.String()
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

func (t tokenResponse) toModel() models.SpotifyToken {
	return models.SpotifyToken{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Scope:        t.Scope,
		ExpiresAt:    time.Now().UTC().Add(time.Duration(t.ExpiresIn-30) * time.Second),
	}
}
