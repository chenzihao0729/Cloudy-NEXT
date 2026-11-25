// Share the Cloudy-NEXT logo with the site's favicon and preview image.
export function AppIcon(props: { size?: number; class?: string }) {
  return (
    <img
      class={props.class}
      src="/icon.png"
      width={props.size ?? 32}
      height={props.size ?? 32}
      alt=""
      aria-hidden="true"
    />
  )
}
