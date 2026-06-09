export default function VideoCardSkeleton({ index = 0 }: { index?: number }) {
  const delay = `${index * 60}ms`
  return (
    <div className="flex flex-col overflow-hidden rounded-xl bg-card border border-white/[0.06]"
         style={{ animationDelay: delay }}>
      <div className="aspect-video shimmer" style={{ animationDelay: delay }} />
      <div className="p-2.5 space-y-2">
        <div className="h-3 shimmer rounded w-3/4" style={{ animationDelay: delay }} />
        <div className="h-3 shimmer rounded w-1/2" style={{ animationDelay: delay }} />
        <div className="flex gap-1.5 mt-2">
          <div className="h-3 shimmer rounded-full w-12" style={{ animationDelay: delay }} />
          <div className="h-3 shimmer rounded-full w-16" style={{ animationDelay: delay }} />
        </div>
      </div>
    </div>
  )
}
