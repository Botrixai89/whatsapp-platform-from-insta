/**
 * Centralized Chart.js setup
 * Import this module in components that need charts to ensure registration happens once
 */
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  ArcElement,
  Title,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'

// Register Chart.js components once
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  ArcElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

// Set default options for better tooltip behavior
// This makes tooltips show when hovering near data points, not just exactly on them
ChartJS.defaults.interaction.mode = 'index'
ChartJS.defaults.interaction.intersect = false

// Restrained, theme-neutral styling (mid-grey reads on both dark and light backgrounds)
ChartJS.defaults.font.family = getComputedStyle(document.body).fontFamily || 'Inter, system-ui, sans-serif'
ChartJS.defaults.font.size = 11
ChartJS.defaults.color = '#8a8f98'
ChartJS.defaults.borderColor = 'rgba(138, 143, 152, 0.14)'
ChartJS.defaults.plugins.legend.align = 'end'
ChartJS.defaults.plugins.legend.labels.usePointStyle = true
ChartJS.defaults.plugins.legend.labels.pointStyle = 'circle'
ChartJS.defaults.plugins.legend.labels.boxWidth = 6
ChartJS.defaults.plugins.legend.labels.boxHeight = 6
ChartJS.defaults.plugins.legend.labels.padding = 16
ChartJS.defaults.plugins.tooltip.backgroundColor = 'rgba(17, 17, 19, 0.95)'
ChartJS.defaults.plugins.tooltip.borderColor = 'rgba(255, 255, 255, 0.08)'
ChartJS.defaults.plugins.tooltip.borderWidth = 1
ChartJS.defaults.plugins.tooltip.padding = 10
ChartJS.defaults.plugins.tooltip.cornerRadius = 8
ChartJS.defaults.plugins.tooltip.usePointStyle = true
ChartJS.defaults.plugins.tooltip.boxPadding = 4
ChartJS.defaults.elements.line.borderWidth = 2
ChartJS.defaults.elements.point.radius = 0
ChartJS.defaults.elements.point.hoverRadius = 4
ChartJS.defaults.elements.point.hitRadius = 8
ChartJS.defaults.elements.bar.borderRadius = 4
ChartJS.defaults.datasets.bar.maxBarThickness = 28
ChartJS.defaults.scale.grid.drawTicks = false
ChartJS.defaults.scale.ticks.padding = 8
// Cartesian-only options; the shared scale defaults are typed as a union with radial scales
Object.assign(ChartJS.defaults.scale.ticks, { maxRotation: 0, autoSkipPadding: 16 })
Object.assign(ChartJS.defaults.scale, { border: { display: false } })

// Re-export chart components for convenience
export { Line, Bar, Pie, Doughnut } from 'vue-chartjs'
export { ChartJS }

/** Formats a YYYY-MM-DD key as a short axis label, e.g. "Oct 4". */
export function shortDayLabel(isoDate: string): string {
  const [y, m, d] = isoDate.split('-').map(Number)
  if (!y || !m || !d) return isoDate
  return new Date(y, m - 1, d).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}
