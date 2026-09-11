const tokens = require('./design-tokens.json');

module.exports = {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: tokens.colors,
      borderRadius: tokens.borderRadius,
    },
  },
  plugins: [],
};