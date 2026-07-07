type PeapodLogoProps = {
  className?: string;
  title?: string;
};

export function PeapodLogo({ className = "", title }: PeapodLogoProps) {
  return (
    <img
      className={`peapod-logo ${className}`.trim()}
      src="/peapod-logo.svg?v=pea"
      alt={title || ""}
      aria-hidden={title ? undefined : true}
      draggable={false}
    />
  );
}
