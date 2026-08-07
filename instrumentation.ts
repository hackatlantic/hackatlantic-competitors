import { registerOTel } from "@vercel/otel";

export function register() {
  registerOTel({
    serviceName: "hackatlantic-ats-web",
    attributes: {
      "deployment.environment.name":
        process.env.DEPLOYMENT_ENVIRONMENT ?? process.env.VERCEL_ENV ?? "development",
      "service.version":
        process.env.APP_VERSION ?? process.env.VERCEL_GIT_COMMIT_SHA ?? "dev",
      "vcs.ref.head.revision":
        process.env.GIT_SHA ?? process.env.VERCEL_GIT_COMMIT_SHA ?? "unknown",
    },
  });
}
