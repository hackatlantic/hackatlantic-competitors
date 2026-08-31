import Link from "next/link";

export function LegalFooter() {
  return (
    <footer className="legal-footer">
      <p>© 2026 HackAtlantic</p>
      <nav aria-label="Legal and support links">
        <Link href="/privacy">Privacy</Link>
        <Link href="/terms">Terms</Link>
        <a href="mailto:team@hackatlantic.ca">Contact</a>
        <a href="https://www.hackatlantic.ca/">Main website</a>
      </nav>
    </footer>
  );
}
