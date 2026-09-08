import lightSVG from '@/assets/favicon-light.svg?url&no-inline'
import darkSVG from '@/assets/favicon-dark.svg?url&no-inline'
import lightICO from '@/assets/favicon-light.ico?url&no-inline'
import darkICO from '@/assets/favicon-dark.ico?url&no-inline'

const icons = {
  light: { svg: lightSVG, ico: lightICO },
  dark: { svg: darkSVG, ico: darkICO },
}

// Update both formats: browsers can choose either, and ICO cannot run media queries.
// Vite emits content-hashed URLs so changing the artwork also invalidates caches.
export function applyFavicon(theme: 'light' | 'dark') {
  for (const format of ['ico', 'svg'] as const) {
    const link = document.querySelector<HTMLLinkElement>(`#favicon-${format}`)
    if (link && link.getAttribute('href') !== icons[theme][format]) {
      link.setAttribute('href', icons[theme][format])
    }
  }
}
