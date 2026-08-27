import { ApplicationEntry } from "@/components/application-entry";

export default function ApplicationPreviewPage() {
  return (
    <main className="page portal-page application-preview-page">
      <div className="signed-in-home portal-workspace application-flow-workspace">
        <ApplicationEntry previewMode />
      </div>
    </main>
  );
}
