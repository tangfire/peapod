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
      <path className="zephyr-logo-pipeline" d="M18 21h19c6.4 0 11 4.5 11 10.7V43" />
      <path className="zephyr-logo-wind zephyr-logo-wind-back" d="M13 25c8.8-8.1 22.5-8.9 33.2-2.2 5.4 3.4 5.6 9.5.5 13.2-3.7 2.7-9.5 3.1-15.1 1.4" />
      <path className="zephyr-logo-wind zephyr-logo-wind-front" d="M15 35c7.2-4.9 18.4-5.1 27.8-.8 5.6 2.6 5.4 8.2-.5 10.7-4.5 1.9-10.9 1.5-17.4-.5" />
      <path className="zephyr-logo-monitor" d="M17 43h8l3.3-6.9 4.2 13.4 4-9.2H47" />
      <path className="zephyr-logo-flare" d="M39.2 14.8 25.5 33.4h10.2l-4.8 15.8 14.8-21.6H35.2l4-12.8Z" />
      <circle className="zephyr-logo-node" cx="18" cy="21" r="3.5" />
      <circle className="zephyr-logo-node" cx="48" cy="31.7" r="3.5" />
      <circle className="zephyr-logo-node" cx="48" cy="43" r="3.5" />
    </svg>
  );
}
