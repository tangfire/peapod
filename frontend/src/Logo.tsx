type ZephyrLogoProps = {
  className?: string;
  title?: string;
};

export function ZephyrLogo({ className = "", title }: ZephyrLogoProps) {
  return (
    <img
      className={`zephyr-logo ${className}`.trim()}
      src="/zephyr-logo.svg"
      alt={title || ""}
      aria-hidden={title ? undefined : true}
      draggable={false}
    />
  );
}
