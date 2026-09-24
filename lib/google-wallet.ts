// Never navigate to an arbitrary URL returned by a misconfigured API/proxy.
export function navigateToGoogleWallet(saveUrl: string) {
  const url = new URL(saveUrl);
  if (url.origin !== "https://pay.google.com" || !/^\/gp\/v\/save\/[A-Za-z0-9_.-]+$/.test(url.pathname) || url.search || url.hash || url.username || url.password) {
    throw new Error("Invalid Google Wallet save URL");
  }
  window.location.assign(url.href);
}
