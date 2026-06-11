import { Link } from 'react-router-dom'

type BrandLogoProps = {
  className?: string
  linked?: boolean
  showTagline?: boolean
  size?: 'nav' | 'sidebar' | 'hero'
}

export default function BrandLogo({
  className = '',
  linked = true,
  showTagline = false,
  size = 'nav',
}: BrandLogoProps) {
  const content = (
    <span className={`brand-logo3d brand-logo3d--${size} ${className}`} aria-label="KaraXXX - Adult Playground">
      <span className="brand-logo3d__word" aria-hidden="true">
        <span className="brand-logo3d__piece brand-logo3d__piece--kara" data-text="Kara">Kara</span>
        <span className="brand-logo3d__piece brand-logo3d__piece--xxx" data-text="XXX">XXX</span>
      </span>
      {showTagline && <span className="brand-logo3d__tagline">Adult Playground</span>}
    </span>
  )

  if (!linked) return content

  return (
    <Link viewTransition to="/" className="inline-flex flex-shrink-0 select-none" aria-label="KaraXXX - Adult Playground home">
      {content}
    </Link>
  )
}
