declare module 'qrcode.react' {
  import type { ComponentType, SVGProps } from 'react'

  export interface QRCodeSVGProps extends SVGProps<SVGSVGElement> {
    value: string
    size?: number
    level?: 'L' | 'M' | 'Q' | 'H'
  }

  export const QRCodeSVG: ComponentType<QRCodeSVGProps>
}
