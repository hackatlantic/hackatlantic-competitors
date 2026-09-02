import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";

const root = resolve(import.meta.dirname, "..", "..");
const fence = "`".repeat(3);
const pairs = [["[", "]"], ["(", ")"], ["{", "}"]];

function diagramsIn(file) {
  const markdown = readFileSync(resolve(root, file), "utf8");
  return markdown
    .split(`${fence}mermaid`)
    .slice(1)
    .map((block) => block.split(fence)[0].trim());
}

function assertBalanced(diagram, file) {
  for (const [open, close] of pairs) {
    const opens = [...diagram].filter((character) => character === open).length;
    const closes = [...diagram].filter((character) => character === close).length;
    assert.equal(opens, closes, `${file} has unbalanced ${open}${close}`);
  }
}

test("public architecture documents contain structurally valid Mermaid blocks", () => {
  const files = ["README.md", "ARCHITECTURE.md", "docs/PLATFORM_ENGINEERING.md"];
  for (const file of files) {
    const diagrams = diagramsIn(file);
    assert.ok(diagrams.length > 0, `${file} should contain a Mermaid diagram`);
    for (const diagram of diagrams) {
      assert.match(diagram, /^(flowchart|sequenceDiagram)\b/, `${file} has an unsupported diagram declaration`);
      assertBalanced(diagram, file);
    }
  }
  assert.ok(
    diagramsIn("ARCHITECTURE.md").some((diagram) => diagram.startsWith("sequenceDiagram")),
    "ARCHITECTURE.md should document the scanner redemption sequence",
  );
});
