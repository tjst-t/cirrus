import type { Config } from 'tailwindcss'

const config: Config = {
  content: [
    './index.html',
    './src/**/*.{ts,tsx}',
  ],
  theme: {
    extend: {
      colors: {
        accent: {
          DEFAULT: '#1877f2',
          hover: '#1466d4',
          subtle: '#e7f3ff',
        },
        success: '#31a24c',
        warning: '#f0960a',
        danger: '#e4363a',
      },
      fontFamily: {
        sans: ['Geist', 'Noto Sans JP', 'system-ui', 'sans-serif'],
      },
      fontWeight: {
        normal: '400',
        medium: '500',
        semibold: '600',
      },
      borderRadius: {
        DEFAULT: '4px',
        sm: '2px',
        md: '4px',
        lg: '6px',
        xl: '8px',  // capped at 8px per design system
      },
      spacing: {
        1: '4px',
        2: '8px',
        3: '12px',
        4: '16px',
        5: '24px',
        6: '32px',
      },
    },
  },
  plugins: [],
}

export default config
