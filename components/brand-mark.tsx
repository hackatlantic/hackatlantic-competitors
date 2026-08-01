import Link from "next/link";

type BrandMarkProps = {
  compact?: boolean;
  href?: string;
  inverse?: boolean;
};

export function BrandMark({ compact = false, href = "/", inverse = false }: BrandMarkProps) {
  return (
    <Link
      aria-label="HackAtlantic application portal"
      className={`brand-mark${inverse ? " brand-mark-inverse" : ""}`}
      href={href}
    >
      {compact ? null : (
        <span>
          <strong>HackAtlantic</strong>
          <small>Applications</small>
        </span>
      )}
    </Link>
  );
}
