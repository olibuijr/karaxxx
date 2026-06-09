export default function VideoCardSkeleton({ index = 0 }: { index?: number }) {
  const delay = `${index * 50}ms`
  return (
    <div className="flex flex-col overflow-hidden rounded-lg bg-card border border-border animate-pulse"
         style={{ animationDelay: delay }}>
      <div className="aspect-video bg-card-hover" />
      <div className="p-2.5 space-y-2">
        <div className="h-3 bg-card-hover rounded w-3/4" />
        <div className="h-3 bg-card-hover rounded w-1/2" />
        <div className="flex gap-1.5 mt-2">
          <div className="h-3 bg-card-hover rounded-full w-12" />
          <div className="h-3 bg-card-hover rounded-full w-16" />
        </div>
      </div>
    </div>
  )
}
