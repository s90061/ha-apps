import React from 'react';

interface BrandMarkProps {
  size?: number;
}

/** Flat geometric projection-frame mark for PiStream */
export const BrandMark: React.FC<BrandMarkProps> = ({ size = 28 }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 28 28"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    aria-hidden="true"
    className="flex-none"
  >
    <rect
      x="1.5"
      y="4.5"
      width="25"
      height="19"
      rx="4"
      fill="#232325"
      stroke="#ededed"
      strokeOpacity="0.16"
      strokeWidth="1.5"
    />
    <path d="M11.5 10.25L18.25 14L11.5 17.75V10.25Z" fill="#e2a336" />
  </svg>
);
