import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa'

/**
 * Bump whenever the icon art changes.
 *
 * Vite does not fingerprint files in public/, so the icons keep stable names
 * across builds. iOS caches the home screen icon by URL and will happily show
 * last year's art forever; this query gives it a URL it has never seen.
 */
const ICON_VERSION = '2'

/** Substitutes ICON_VERSION into index.html so the number lives in one place. */
function iconVersion(): Plugin {
  return {
    name: 'rhythmiq-icon-version',
    transformIndexHtml(html) {
      return html.replaceAll('%ICON_VERSION%', ICON_VERSION)
    }
  }
}

export default defineConfig({
  plugins: [
    react(),
    iconVersion(),
    VitePWA({
      registerType: 'autoUpdate',
      includeAssets: ['icon.png', 'icon-192.png', 'apple-touch-icon.png'],
      manifest: {
        name: 'RhythmIQ',
        short_name: 'RhythmIQ',
        description: 'Your Spotify listening, by the week, the month, and the year.',
        theme_color: '#0c0b10',
        background_color: '#0c0b10',
        id: '/',
        scope: '/',
        start_url: '/',
        display: 'standalone',
        display_override: ['standalone', 'minimal-ui'],
        orientation: 'portrait',
        categories: ['music', 'lifestyle'],
        icons: [
          { src: `icon-192.png?v=${ICON_VERSION}`, sizes: '192x192', type: 'image/png', purpose: 'any' },
          { src: `icon.png?v=${ICON_VERSION}`, sizes: '512x512', type: 'image/png', purpose: 'any' },
          // Kept separate from 'any': maskable art needs its own safe-zone
          // padding or Android crops the logo.
          {
            src: `icon-maskable-192.png?v=${ICON_VERSION}`,
            sizes: '192x192',
            type: 'image/png',
            purpose: 'maskable'
          },
          {
            src: `icon-maskable-512.png?v=${ICON_VERSION}`,
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable'
          }
        ]
      },
      workbox: {
        globPatterns: ['**/*.{js,css,html,svg,png,ico}'],
        navigateFallbackDenylist: [/^\/api\//],
        // The icons are precached under their bare names, so ?v= would miss
        // every time. Stripping it still yields the current art, because
        // precache entries are revisioned by content. Extends the default list
        // rather than replacing it.
        ignoreURLParametersMatching: [/^utm_/, /^fbclid$/, /^v$/],
        runtimeCaching: [
          {
            // Webfonts and artwork are cross-origin, so they need explicit
            // runtime caching to survive offline.
            urlPattern: ({ url }) =>
              url.origin === 'https://fonts.googleapis.com' ||
              url.origin === 'https://fonts.gstatic.com',
            handler: 'CacheFirst',
            options: {
              cacheName: 'font-cache',
              expiration: { maxEntries: 20, maxAgeSeconds: 60 * 60 * 24 * 365 },
              cacheableResponse: { statuses: [0, 200] }
            }
          },
          {
            urlPattern: ({ url }) => url.origin === 'https://i.scdn.co',
            handler: 'CacheFirst',
            options: {
              cacheName: 'artwork-cache',
              expiration: { maxEntries: 200, maxAgeSeconds: 60 * 60 * 24 * 30 },
              cacheableResponse: { statuses: [0, 200] }
            }
          },
          {
            urlPattern: ({ url, request }) =>
              request.method === 'GET' &&
              url.pathname.startsWith('/api/') &&
              !url.pathname.startsWith('/api/auth/'),
            handler: 'NetworkFirst',
            options: {
              cacheName: 'api-cache',
              expiration: {
                maxEntries: 40,
                maxAgeSeconds: 60 * 60
              },
              networkTimeoutSeconds: 10
            }
          }
        ]
      }
    })
  ],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  }
})
