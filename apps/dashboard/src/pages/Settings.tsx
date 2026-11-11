import { Btn, Card, PageTitle } from "../components/Ui";
import { useUi } from "../stores/auth";

export default function SettingsPage() {
  const setToken = useUi((s) => s.setToken);
  return (
    <>
      <PageTitle title="Settings" />
      <Card title="Session">
        <p className="mb-3 text-sm text-slate-400">Your token is kept in this browser tab only (session storage) and is cleared when you close it.</p>
        <Btn onClick={() => setToken("")}>Sign out</Btn>
      </Card>
      <Card title="Configuration" className="mt-4">
        <ul className="list-disc space-y-1 pl-5 text-sm text-slate-300">
          <li>Recovery mode default, tokens and agent secret are set in the <code>.env</code> file (see README).</li>
          <li>Recovery behaviour per service is managed on the Recovery Policies page.</li>
          <li>Use REFORGE_VIEWER_TOKEN to give read-only access to colleagues.</li>
        </ul>
      </Card>
    </>
  );
}
