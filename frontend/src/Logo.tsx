type ZephyrLogoProps = {
  className?: string;
  title?: string;
};

export function ZephyrLogo({ className = "", title }: ZephyrLogoProps) {
  return (
    <svg
      className={`zephyr-logo ${className}`.trim()}
      viewBox="0 0 64 64"
      role={title ? "img" : undefined}
      aria-hidden={title ? undefined : true}
      focusable="false"
    >
      {title && <title>{title}</title>}
      <circle className="zephyr-logo-core" cx="32" cy="32" r="27" />
      <path className="zephyr-logo-wind" d="M17 30c8.5-8 22.8-8 31 0" />
      <path className="zephyr-logo-wind zephyr-logo-wind-soft" d="M20 38c6.6-4.9 17.4-4.9 24 0" />
      <path className="zephyr-logo-monitor" d="M20 45h7l3-6 4 10 3.6-7H45" />
      <circle className="zephyr-logo-node" cx="17" cy="30" r="4" />
      <circle className="zephyr-logo-node" cx="48" cy="30" r="4" />
    </svg>
  );
}
