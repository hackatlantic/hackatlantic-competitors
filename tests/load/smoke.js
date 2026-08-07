import http from "k6/http";
import { check } from "k6";

export const options = {
  vus: 5,
  duration: "30s",
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500"],
    checks: ["rate>0.99"],
  },
};

const baseURL = __ENV.API_BASE_URL;

export default function smokeLoad() {
  const health = http.get(`${baseURL}/healthz`, { tags: { route: "/healthz" } });
  check(health, {
    "health is 200": (response) => response.status === 200,
    "health is ok": (response) => response.json("status") === "ok",
  });

  const version = http.get(`${baseURL}/versionz`, { tags: { route: "/versionz" } });
  check(version, {
    "version is 200": (response) => response.status === 200,
    "version has git SHA": (response) => Boolean(response.json("gitSha")),
  });
}
