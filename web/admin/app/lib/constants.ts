export const QUEUE_BACKLOG_LOW_WATERMARK = 10
export const QUEUE_BACKLOG_HIGH_WATERMARK = 1000

export type QueueHealthLevel = 'healthy' | 'warning' | 'critical'

export function queueHealth(backlog: number): {level: QueueHealthLevel; label: string} {
  if (backlog >= QUEUE_BACKLOG_HIGH_WATERMARK) {
    return {level: 'critical', label: 'Critical load'}
  }
  if (backlog > QUEUE_BACKLOG_LOW_WATERMARK) {
    return {level: 'warning', label: 'Under pressure'}
  }
  return {level: 'healthy', label: 'Healthy'}
}
