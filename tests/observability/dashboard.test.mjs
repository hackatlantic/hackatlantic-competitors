import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const dashboard = JSON.parse(readFileSync(new URL("../../observability/grafana/dashboards/api-overview.json", import.meta.url), "utf8"));
const expressions = dashboard.panels.flatMap((panel) => panel.targets.map((target) => target.expr));

test("dashboard IDs are unique and all panels select the API and environment", () => {
  assert.equal(new Set(dashboard.panels.map((panel) => panel.id)).size, dashboard.panels.length);
  for (const expr of expressions) {
    assert.ok(expr.includes('service_name="hackatlantic-ats-api"'));
    assert.ok(expr.includes('deployment_environment_name=~"$environment"'));
  }
});

test("scanner panels use the registered API routes, not raw paths", () => {
  const router = readFileSync(new URL("../../api/internal/httpapi/handler.go", import.meta.url), "utf8");
  const scanner = dashboard.panels.find((panel) => panel.id === 7);
  for (const target of scanner.targets) {
    const route = /http_route="([^"]+)"/.exec(target.expr)?.[1];
    assert.ok(route);
    assert.ok(router.includes('mux.HandleFunc("' + route + '"'), route);
  }
});

test("expressions have balanced delimiters (not a substitute for live PromQL evaluation)", () => {
  for (const expr of expressions) {
    const stack = [];
    for (const character of expr.replace(/"(?:[^"\\]|\\.)*"/g, '""')) {
      if ("([{".includes(character)) stack.push(character);
      if (")]}".includes(character)) {
        assert.equal(stack.pop(), { ")": "(", "]": "[", "}": "{" }[character], expr);
      }
    }
    assert.equal(stack.length, 0, expr);
  }
});

test("non-5xx panel does not invent success when there is no traffic", () => {
  const panel = dashboard.panels.find((item) => item.id === 1);
  assert.ok(panel.targets[0].expr.includes("or 0 * sum(rate("));
  assert.ok(!panel.targets[0].expr.includes("clamp_min"));
  assert.equal(panel.fieldConfig.defaults.noValue, "No traffic / no data");
});
