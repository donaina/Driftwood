/* Theme, on the site.

   The dashboard and the site share one attribute (`data-theme` on <html>) and
   one storage key, so a reader who chose dark in the app is not flashed light
   when they land here — and their choice carries back. That only works because
   both write the same key, which is worth stating out loud: a second key would
   look like it worked and would silently disagree with the other page.

   `data-theme` goes on <html> for the same reason it does in the dashboard: a
   custom property's var() reference is substituted at computed-value time on
   the element that declares it, so an attribute on <body> would be read after
   the tokens had already resolved. */

export type Theme = 'light' | 'dark';

export const THEME_KEY = 'driftwood-theme';

/** The theme actually in effect, which is the stored choice if there is one and
 *  the OS preference otherwise. */
export function currentTheme(): Theme {
  try {
    const stored = localStorage.getItem(THEME_KEY);
    if (stored === 'light' || stored === 'dark') return stored;
  } catch {
    /* storage unavailable — fall through to the OS preference */
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function applyTheme(theme: Theme): void {
  document.documentElement.setAttribute('data-theme', theme);
  try {
    localStorage.setItem(THEME_KEY, theme);
  } catch {
    /* The choice applies to this page either way; it just will not be
       remembered. Not worth failing a click over. */
  }
}
