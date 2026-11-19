/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        deepest: '#0f0f10',
        background: '#141415',
        surface: '#19191b',
        raised: '#232325',
        hover: '#2c2c30',
        border: 'rgba(237, 237, 237, 0.08)',
        'border-strong': 'rgba(237, 237, 237, 0.14)',
        'ink-primary': '#ededed',
        'ink-secondary': '#a1a1a6',
        'ink-muted': '#85858b',
        accent: '#e2a336',
        'accent-hover': '#eab24d',
        positive: '#4cc38a',
        negative: '#e5484d',
      },
      fontFamily: {
        sans: ['"Inter"', '"Noto Sans TC"', '"PingFang TC"', '-apple-system', 'BlinkMacSystemFont', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
      transitionTimingFunction: {
        cinema: 'cubic-bezier(0.22, 1, 0.36, 1)',
      },
    },
  },
  plugins: [],
};
