/**
 * The page takes its accent colour from the artwork of whoever is currently
 * at no.1, so switching periods re-tints the whole spread. If the image can't
 * be read (CORS, offline, missing artwork) callers fall back to the default
 * accent defined in CSS.
 */

export interface Accent {
  hue: number
  saturation: number
  lightness: number
}

const SAMPLE_SIZE = 24
const HUE_BUCKETS = 24

export async function extractAccent(imageUrl: string): Promise<Accent | null> {
  if (!imageUrl) {
    return null
  }

  let pixels: Uint8ClampedArray
  try {
    const image = await loadImage(imageUrl)
    const canvas = document.createElement('canvas')
    canvas.width = SAMPLE_SIZE
    canvas.height = SAMPLE_SIZE

    const context = canvas.getContext('2d', { willReadFrequently: true })
    if (!context) {
      return null
    }
    context.drawImage(image, 0, 0, SAMPLE_SIZE, SAMPLE_SIZE)
    pixels = context.getImageData(0, 0, SAMPLE_SIZE, SAMPLE_SIZE).data
  } catch {
    // Tainted canvas or a failed load; the CSS default accent stays in place.
    return null
  }

  const weights = new Array<number>(HUE_BUCKETS).fill(0)
  const saturations = new Array<number>(HUE_BUCKETS).fill(0)
  const lightnesses = new Array<number>(HUE_BUCKETS).fill(0)

  for (let i = 0; i < pixels.length; i += 4) {
    if (pixels[i + 3] < 128) {
      continue
    }
    const [h, s, l] = rgbToHsl(pixels[i], pixels[i + 1], pixels[i + 2])

    // Ignore near-black, near-white, and washed-out pixels: they carry no hue.
    if (l < 0.12 || l > 0.92 || s < 0.18) {
      continue
    }

    // Favour colours that are both vivid and mid-toned, which survive being
    // used as ink on a dark page.
    const weight = s * (1 - Math.abs(l - 0.5) * 1.2)
    const bucket = Math.min(HUE_BUCKETS - 1, Math.floor((h / 360) * HUE_BUCKETS))
    weights[bucket] += weight
    saturations[bucket] += s * weight
    lightnesses[bucket] += l * weight
  }

  let best = -1
  let bestWeight = 0
  for (let bucket = 0; bucket < HUE_BUCKETS; bucket += 1) {
    if (weights[bucket] > bestWeight) {
      bestWeight = weights[bucket]
      best = bucket
    }
  }
  if (best < 0 || bestWeight <= 0) {
    return null
  }

  const hue = (best + 0.5) * (360 / HUE_BUCKETS)

  // Push the result into a range that stays legible against the dark page,
  // regardless of how muted the source photograph was.
  return {
    hue: Math.round(hue),
    saturation: Math.round(clamp(saturations[best] / bestWeight, 0.55, 0.95) * 100),
    lightness: Math.round(clamp(lightnesses[best] / bestWeight, 0.55, 0.72) * 100)
  }
}

function loadImage(url: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.crossOrigin = 'anonymous'
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error('image failed to load'))
    image.src = url
  })
}

function rgbToHsl(r: number, g: number, b: number): [number, number, number] {
  const red = r / 255
  const green = g / 255
  const blue = b / 255

  const max = Math.max(red, green, blue)
  const min = Math.min(red, green, blue)
  const lightness = (max + min) / 2

  if (max === min) {
    return [0, 0, lightness]
  }

  const delta = max - min
  const saturation = lightness > 0.5 ? delta / (2 - max - min) : delta / (max + min)

  let hue: number
  if (max === red) {
    hue = ((green - blue) / delta + (green < blue ? 6 : 0)) / 6
  } else if (max === green) {
    hue = ((blue - red) / delta + 2) / 6
  } else {
    hue = ((red - green) / delta + 4) / 6
  }

  return [hue * 360, saturation, lightness]
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}
