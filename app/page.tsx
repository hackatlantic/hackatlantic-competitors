import {
  Show,
  SignInButton,
  SignUpButton,
  UserButton
} from "@clerk/nextjs";

export default function Home() {
  return (
    <main className="page">
      <nav className="nav" aria-label="Account">
        <Show when="signed-out">
          <SignInButton>
            <button className="button secondary" type="button">
              Sign in
            </button>
          </SignInButton>
          <SignUpButton>
            <button className="button primary" type="button">
              Sign up
            </button>
          </SignUpButton>
        </Show>
        <Show when="signed-in">
          <UserButton />
        </Show>
      </nav>

      <section className="intro">
        <p className="eyebrow">Next.js skeleton</p>
        <h1>HackAtlantic Competitors</h1>
        <p>
          Start building your app in <code>app/page.tsx</code>. Shared layout
          and global styles live under <code>app/</code>.
        </p>
      </section>
    </main>
  );
}
