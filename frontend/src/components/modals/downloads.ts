import state from '../../state'
import type { DownloadItem } from '../../types'
import { icon } from '../../utils/icons'
import { escapeHtml } from '../../utils/html'

function formatKind(kind: DownloadItem['kind']): string {
	switch (kind) {
		case 'stt-model':
			return 'STT Model'
		case 'llm-model':
			return 'LLM Model'
		case 'llama-server':
			return 'Llama Server'
		default:
			return kind
	}
}

function formatBytes(bytes: number): string {
	if (!bytes || bytes <= 0) return '—'
	const units = ['B', 'KB', 'MB', 'GB']
	let i = 0
	let val = bytes
	while (val >= 1024 && i < units.length - 1) {
		val /= 1024
		i++
	}
	return `${val.toFixed(val >= 10 || i === 0 ? 0 : 1)} ${units[i]}`
}

function statusLabel(item: DownloadItem): string {
	switch (item.status) {
		case 'queued':
			return 'Queued'
		case 'downloading':
			return 'Downloading'
		case 'completed':
			return 'Completed'
		case 'error':
			return item.error ? `Error: ${item.error}` : 'Error'
		default:
			return item.status
	}
}

export function renderDownloadsModal(container: HTMLElement): void {
	let card = container.querySelector('.modal-card-downloads')
	if (!card) {
		container.innerHTML = `
			<div class="modal-card modal-card-downloads">
				<div class="modal-header">
					<h2 class="modal-heading">Downloads</h2>
					<button id="downloads-close" class="btn-icon">${icon('x', 16)}</button>
				</div>
				<div class="modal-body downloads-body">
					<div id="downloads-content"></div>
				</div>
				<div class="modal-footer">
					<button id="downloads-dismiss" class="btn-secondary">Close</button>
				</div>
			</div>
		`

		card = container.querySelector('.modal-card-downloads')
		container.querySelector('#downloads-close')?.addEventListener('click', () => state.closeModal())
		container.querySelector('#downloads-dismiss')?.addEventListener('click', () => state.closeModal())
	}

	const downloads = state.get('downloads')
	const visible = downloads
		.filter((d) => (d.status !== 'completed' && d.status !== 'error') || d.completedInSession)
		.sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())

	const rows = visible.map((item) => {
		const percent = Math.min(100, Math.max(0, item.percent || 0))
		const isComplete = item.status === 'completed'
		const isError = item.status === 'error'
		const statusIcon = isError
			? icon('alert-circle', 18)
			: isComplete
				? icon('check-circle', 18)
				: icon('loader', 18)

		const iconClasses = ['download-item-icon']
		if (!isComplete && !isError) {
			iconClasses.push('download-item-icon-spin')
		}

		const safeKind = escapeHtml(formatKind(item.kind))
		const safeLabel = escapeHtml(item.label)
		const safeStatus = escapeHtml(statusLabel(item))
		const safeBytes = escapeHtml(`${formatBytes(item.bytesDownloaded)} downloaded`)
		const safePercentText = escapeHtml(`${percent.toFixed(percent >= 10 ? 0 : 1)}%`)

		return `
			<li class="download-item ${isComplete ? 'download-item-complete' : ''} ${isError ? 'download-item-error' : ''}">
				<div class="${iconClasses.join(' ')}">${statusIcon}</div>
				<div class="download-item-body">
					<div class="download-item-header">
						<div>
							<div class="download-item-label">${safeKind}</div>
							<div class="download-item-name">${safeLabel}</div>
						</div>
						<div class="download-item-meta">${safeStatus}</div>
					</div>
					<div class="download-progress">
						<div class="download-progress-fill" style="width:${percent}%"></div>
					</div>
					<div class="download-stat-row">
						<span>${safeBytes}</span>
						<span>${safePercentText}</span>
					</div>
				</div>
			</li>
		`
	}).join('')

	const contentArea = container.querySelector('#downloads-content')
	if (contentArea) {
		contentArea.innerHTML = visible.length === 0
			? '<div class="download-empty">No downloads this session yet.</div>'
			: `<ul class="download-list">${rows}</ul>`
	}
}
