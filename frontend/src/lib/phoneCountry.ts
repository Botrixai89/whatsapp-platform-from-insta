// Maps a phone number's dialing prefix to a country name and flag.
// Covers the most common markets; unknown prefixes return null.
const COUNTRIES: Array<[string, string, string]> = [
  ['1', 'United States / Canada', '🇺🇸'], ['7', 'Russia', '🇷🇺'], ['20', 'Egypt', '🇪🇬'], ['27', 'South Africa', '🇿🇦'],
  ['31', 'Netherlands', '🇳🇱'], ['33', 'France', '🇫🇷'], ['34', 'Spain', '🇪🇸'], ['39', 'Italy', '🇮🇹'],
  ['44', 'United Kingdom', '🇬🇧'], ['49', 'Germany', '🇩🇪'], ['52', 'Mexico', '🇲🇽'], ['55', 'Brazil', '🇧🇷'],
  ['60', 'Malaysia', '🇲🇾'], ['61', 'Australia', '🇦🇺'], ['62', 'Indonesia', '🇮🇩'], ['63', 'Philippines', '🇵🇭'],
  ['64', 'New Zealand', '🇳🇿'], ['65', 'Singapore', '🇸🇬'], ['66', 'Thailand', '🇹🇭'], ['81', 'Japan', '🇯🇵'],
  ['82', 'South Korea', '🇰🇷'], ['84', 'Vietnam', '🇻🇳'], ['86', 'China', '🇨🇳'], ['90', 'Turkey', '🇹🇷'],
  ['91', 'India', '🇮🇳'], ['92', 'Pakistan', '🇵🇰'], ['94', 'Sri Lanka', '🇱🇰'], ['234', 'Nigeria', '🇳🇬'],
  ['254', 'Kenya', '🇰🇪'], ['880', 'Bangladesh', '🇧🇩'], ['960', 'Maldives', '🇲🇻'], ['965', 'Kuwait', '🇰🇼'],
  ['966', 'Saudi Arabia', '🇸🇦'], ['968', 'Oman', '🇴🇲'], ['971', 'United Arab Emirates', '🇦🇪'], ['973', 'Bahrain', '🇧🇭'],
  ['974', 'Qatar', '🇶🇦'], ['975', 'Bhutan', '🇧🇹'], ['977', 'Nepal', '🇳🇵'],
]

export function phoneCountry(phone?: string): { name: string; flag: string } | null {
  const digits = (phone || '').replace(/\D/g, '')
  let best: [string, string, string] | null = null
  for (const c of COUNTRIES) {
    if (digits.startsWith(c[0]) && (!best || c[0].length > best[0].length)) best = c
  }
  return best ? { name: best[1], flag: best[2] } : null
}
