/**
 * Icon helper — renders Lucide icons from the `lucide` npm package.
 *
 * Uses lucide's canonical icon data directly instead of hand-copied paths,
 * so every icon is pixel-perfect and stays up-to-date with the package.
 *
 * Usage: icon('settings', 18)  →  SVG string
 */

import {
  AlertTriangle,
  AlertCircle,
  ArrowUpDown,
  AudioWaveform,
  CheckCircle,
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
  Download,
  Eye,
  EyeOff,
  FilePlus,
  FileText,
  Folder,
  FolderInput,
  FolderOpen,
  FolderPlus,
  Home,
  Loader,
  Mic,
  Moon,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Search,
  Send,
  Settings,
  Sparkles,
  Square,
  Sun,
  Trash2,
  X,
} from 'lucide'

// Lucide exports each icon as a nested array: [tag, attrs, children[]]
// children is an array of [tag, attrs] tuples.
type LucideIconNode = [string, Record<string, string | number>, LucideIconNode[]]

function renderNode([tag, attrs, children]: LucideIconNode): string {
  const attrStr = Object.entries(attrs)
    .map(([k, v]) => `${k}="${v}"`)
    .join(' ')
  const inner = (children ?? []).map(renderNode).join('')
  return `<${tag} ${attrStr}>${inner}</${tag}>`
}

function toSvg(icon: LucideIconNode, size: number): string {
  const [tag, attrs, children] = icon
  const overrides = { ...attrs, width: size, height: size }
  return renderNode([tag, overrides, children])
}

// Map kebab-case names to lucide exports
const ICONS: Record<string, LucideIconNode> = {
  'alert-triangle':  AlertTriangle  as unknown as LucideIconNode,
  'alert-circle':    AlertCircle    as unknown as LucideIconNode,
  'arrow-up-down':   ArrowUpDown    as unknown as LucideIconNode,
  'audio-waveform':  AudioWaveform  as unknown as LucideIconNode,
  'check':           Check          as unknown as LucideIconNode,
  'check-circle':    CheckCircle    as unknown as LucideIconNode,
  'chevron-down':    ChevronDown    as unknown as LucideIconNode,
  'chevron-right':   ChevronRight   as unknown as LucideIconNode,
  'copy':            Copy           as unknown as LucideIconNode,
  'download':        Download       as unknown as LucideIconNode,
  'eye':             Eye            as unknown as LucideIconNode,
  'eye-off':         EyeOff         as unknown as LucideIconNode,
  'file-plus':       FilePlus       as unknown as LucideIconNode,
  'file-text':       FileText       as unknown as LucideIconNode,
  'folder':          Folder         as unknown as LucideIconNode,
  'folder-input':    FolderInput    as unknown as LucideIconNode,
  'folder-open':     FolderOpen     as unknown as LucideIconNode,
  'folder-plus':     FolderPlus     as unknown as LucideIconNode,
  'home':            Home           as unknown as LucideIconNode,
  'loader':          Loader         as unknown as LucideIconNode,
  'mic':             Mic            as unknown as LucideIconNode,
  'moon':            Moon           as unknown as LucideIconNode,
  'pencil':          Pencil         as unknown as LucideIconNode,
  'play':            Play           as unknown as LucideIconNode,
  'plus':            Plus           as unknown as LucideIconNode,
  'refresh-cw':      RefreshCw      as unknown as LucideIconNode,
  'search':          Search         as unknown as LucideIconNode,
  'send':            Send           as unknown as LucideIconNode,
  'settings':        Settings       as unknown as LucideIconNode,
  'sparkles':        Sparkles       as unknown as LucideIconNode,
  'square':          Square         as unknown as LucideIconNode,
  'sun':             Sun            as unknown as LucideIconNode,
  'trash-2':         Trash2         as unknown as LucideIconNode,
  'x':               X              as unknown as LucideIconNode,
}

export function icon(name: string, size = 16): string {
  const data = ICONS[name]
  if (!data) {
    console.warn(`[icons] unknown icon: "${name}"`)
    return `<svg width="${size}" height="${size}" viewBox="0 0 24 24"/>`
  }
  return toSvg(data, size)
}
