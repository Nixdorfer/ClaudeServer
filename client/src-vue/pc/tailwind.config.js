/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        'chat-bg': '#18181b',
        'sidebar-bg': '#27272a',
        'input-bg': '#3f3f46',
        'message-user': '#3b82f6',
        'message-ai': '#27272a',
      },
    },
  },
  plugins: [],
}
