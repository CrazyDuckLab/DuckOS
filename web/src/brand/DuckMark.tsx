/**
 * DuckOS V1 — DuckMark V4 (rubber-duck direction).
 *
 * Recreated to match the visual reference — a classic bath-duck silhouette
 * with subtle rubber-toy shading, a small orange beak, a dark eye, a
 * raised tail bump, and a visible wing curve on the body.
 *
 * The component renders the same shape at every size. Below 22 px the
 * wing and beak-shadow drop out so the silhouette stays clean at favicon
 * sizes; the head, body, tail, beak, and eye are always present.
 *
 * Colours stay warm in BOTH themes — the duck is the DuckOS brand, not
 * a theme-coloured element. The gradients live in the embedded <defs>
 * of the SVG so the asset is fully self-contained.
 */
import type { CSSProperties } from "react";

interface Props {
  size?: number;
  className?: string;
  title?: string;
  style?: CSSProperties;
  /** Hide the eye dot. Defaults to true. Hidden automatically below 18 px. */
  withEye?: boolean;
}

export function DuckMark({
  size = 32,
  className,
  title = "DuckOS",
  style,
  withEye = true,
}: Props) {
  const drawEye = withEye && size >= 18;
  const drawWing = size >= 22;
  const drawBeakShadow = size >= 22;
  const drawSheen = size >= 22;
  const drawTail = true;

  return (
    <svg
      role="img"
      aria-label={title}
      viewBox="0 0 64 64"
      width={size}
      height={size}
      className={className}
      style={style}
      focusable="false"
    >
      <title>{title}</title>
      <defs>
        <radialGradient id="duckos-body-grad" cx="40%" cy="35%" r="80%">
          <stop offset="0%" stop-color="#FFE15A" />
          <stop offset="55%" stop-color="#F5C533" />
          <stop offset="100%" stop-color="#C99428" />
        </radialGradient>
        <linearGradient id="duckos-beak-grad" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#FF9244" />
          <stop offset="100%" stop-color="#D7601B" />
        </linearGradient>
        <radialGradient id="duckos-shadow-grad" cx="50%" cy="55%" r="55%">
          <stop offset="0%" stop-color="#000000" stop-opacity="0" />
          <stop offset="100%" stop-color="#000000" stop-opacity="0.15" />
        </radialGradient>
      </defs>

      {/* Tail bump (back-left) */}
      {drawTail && (
        <path
          fill="url(#duckos-body-grad)"
          d="M 6 24 Q 4 20 6 16 Q 10 18 12 22 L 12 30 Q 8 30 6 24 Z"
        />
      )}

      {/* Body + head silhouette */}
      <path
        fill="url(#duckos-body-grad)"
        d="M 14 18 Q 16 10 26 8 Q 38 7 44 14 Q 46 14 48 14 L 56 13 Q 60 13 60 16 Q 60 19 56 20 L 48 21 Q 49 28 49 34 Q 47 46 32 52 Q 18 56 8 50 Q 2 46 2 38 Q 2 30 6 26 Q 10 22 14 22 Q 14 19 14 18 Z"
      />

      {/* Subtle underbody shadow for dimensionality */}
      <ellipse cx="28" cy="50" rx="20" ry="5" fill="url(#duckos-shadow-grad)" />

      {/* Wing detail */}
      {drawWing && (
        <path
          fill="#D89F2C"
          opacity="0.42"
          d="M 22 32 Q 28 30 36 32 Q 40 38 36 42 Q 26 44 22 38 Q 20 34 22 32 Z"
        />
      )}

      {/* Soft sheen across the top of the head */}
      {drawSheen && <ellipse cx="28" cy="14" rx="16" ry="4" fill="#FFFFFF" opacity="0.22" />}

      {/* Beak */}
      <path
        fill="url(#duckos-beak-grad)"
        d="M 48 17 L 60 16 Q 62 16 62 18 Q 62 20 60 21 L 54 22 L 48 21 Z"
      />

      {/* Beak shadow line */}
      {drawBeakShadow && (
        <path fill="#8C3F0E" opacity="0.45" d="M 49 19 L 60 18.7" />
      )}

      {/* Eye */}
      {drawEye && (
        <>
          <circle cx="34" cy="14" r="2.2" fill="#1A1A1A" />
          <circle cx="34.7" cy="13.3" r="0.7" fill="#FFFFFF" />
        </>
      )}
    </svg>
  );
}

export function DuckWordmark({ size = 36 }: { size?: number }) {
  return (
    <svg
      role="img"
      aria-label="DuckOS"
      viewBox="0 0 110 28"
      width={size * 3}
      height={size}
      focusable="false"
    >
      <text
        x="0"
        y="22"
        fontFamily="inherit"
        fontSize="22"
        fontWeight={600}
        letterSpacing="-0.5"
        fill="currentColor"
      >
        DuckOS
      </text>
    </svg>
  );
}

export default DuckMark;