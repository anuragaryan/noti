/**
 * Shared scroll helpers.
 */

export const AUTO_SCROLL_THRESHOLD_PX = 80

export function isNearBottom(el: HTMLElement, thresholdPx: number = AUTO_SCROLL_THRESHOLD_PX): boolean {
  const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight
  return distanceFromBottom <= thresholdPx
}

export function scrollToBottom(el: HTMLElement): void {
  requestAnimationFrame(() => {
    el.scrollTop = el.scrollHeight
  })
}
