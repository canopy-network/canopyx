type GapRangesDisplayProps = {
  gap_ranges_count?: number
  largest_gap_start?: number
  largest_gap_end?: number
  missing_blocks_count?: number
}

export function GapRangesDisplay({
  gap_ranges_count = 0,
  largest_gap_start,
  largest_gap_end,
  missing_blocks_count = 0,
}: GapRangesDisplayProps) {
  // No gaps - fully indexed
  if (gap_ranges_count === 0) {
    return (
      <div className="card">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-full bg-emerald-500/20">
            <svg className="h-5 w-5 text-emerald-400" fill="currentColor" viewBox="0 0 20 20">
              <path
                fillRule="evenodd"
                d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z"
                clipRule="evenodd"
              />
            </svg>
          </div>
          <div>
            <div className="text-sm font-medium text-emerald-200">No gaps detected</div>
            <div className="text-xs text-slate-400">All blocks are fully indexed</div>
          </div>
        </div>
      </div>
    )
  }

  const largestGapSize =
    largest_gap_start && largest_gap_end ? largest_gap_end - largest_gap_start + 1 : 0

  return (
    <div className="card">
      <div className="card-header">
        <h3 className="card-title">Gap Summary</h3>
        <span className="badge-warning">{gap_ranges_count} gaps</span>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        {/* Total Missing Blocks */}
        <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-4">
          <div className="text-xs font-medium text-slate-400">Total Missing</div>
          <div className="mt-2 flex items-baseline gap-2">
            <span className="font-mono text-2xl font-bold text-amber-400">
              {missing_blocks_count.toLocaleString()}
            </span>
            <span className="text-sm text-slate-500">blocks</span>
          </div>
        </div>

        {/* Gap Count */}
        <div className="rounded-lg border border-slate-800 bg-slate-900/50 p-4">
          <div className="text-xs font-medium text-slate-400">Gap Ranges</div>
          <div className="mt-2 flex items-baseline gap-2">
            <span className="font-mono text-2xl font-bold text-indigo-400">{gap_ranges_count}</span>
            <span className="text-sm text-slate-500">ranges</span>
          </div>
        </div>
      </div>

      {/* Largest Gap Details */}
      {largestGapSize > 0 && (
        <div className="mt-4 rounded-lg border border-slate-800 bg-slate-900/30 p-4">
          <div className="flex items-start justify-between">
            <div className="flex-1">
              <div className="text-xs font-medium text-slate-400">
                {gap_ranges_count === 1 ? 'Gap Range' : 'Largest Gap'}
              </div>
              <div className="mt-2 space-y-1">
                <div className="flex items-center gap-2 text-sm">
                  <span className="text-slate-500">Range:</span>
                  <span className="font-mono text-white">
                    {largest_gap_start?.toLocaleString()} - {largest_gap_end?.toLocaleString()}
                  </span>
                </div>
                <div className="flex items-center gap-2 text-sm">
                  <span className="text-slate-500">Size:</span>
                  <span className="font-mono font-semibold text-purple-400">
                    {largestGapSize.toLocaleString()} blocks
                  </span>
                </div>
              </div>
            </div>
            <div className="rounded-lg bg-slate-950/50 px-3 py-1.5">
              <div className="text-xs text-slate-500">
                {((largestGapSize / missing_blocks_count) * 100).toFixed(1)}%
              </div>
              <div className="text-[10px] text-slate-600">of total</div>
            </div>
          </div>
        </div>
      )}

      {/* Info message */}
      <div className="mt-4 flex items-start gap-2 text-xs text-slate-500">
        <svg className="h-4 w-4 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
          />
        </svg>
        <span>
          {gap_ranges_count > 1
            ? 'Multiple gap ranges indicate historical blocks that were missed during initial sync.'
            : 'This gap represents historical blocks that were missed during initial sync.'}
        </span>
      </div>
    </div>
  )
}