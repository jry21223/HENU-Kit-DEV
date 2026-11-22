"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState, useSyncExternalStore, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { formatPortalError, PortalUnauthorizedError } from "@/lib/api/client";
import { authStore } from "@/lib/auth/store";
import { CONTACT_CHANNELS, publishCampusItem, readCampusContactBook, saveCampusContactBook, type ContactBook, type ContactChannel } from "@/lib/campus/gateway";

export const loginForCampus = (path: string) => `/account/login?next=${encodeURIComponent(path)}`;
export const useCampusViewer = () => useSyncExternalStore(authStore.subscribe, authStore.get, authStore.getServer);

export function CampusContactDialog({ path, onSaved }: { path: string; onSaved?: (book: ContactBook) => void }) {
  const { user, ready } = useCampusViewer();
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild><Button variant="outline">联系方式簿</Button></DialogTrigger>
      <DialogContent>
        <DialogTitle>联系方式簿</DialogTitle>
        <DialogDescription>联系方式不会出现在公开单子里。双方同意交换后，所选渠道和备注才会分享给对方。</DialogDescription>
        {ready && user && open ? <ContactForm key={user.uid} path={path} onSaved={onSaved} /> : ready ? (
          <Button asChild className="mt-4"><Link href={loginForCampus(path)}>登录后设置</Link></Button>
        ) : <p role="status" className="mt-4 text-sm">正在核对登录状态…</p>}
      </DialogContent>
    </Dialog>
  );
}

function ContactForm({ path, onSaved }: { path: string; onSaved?: (book: ContactBook) => void }) {
  const [contact, setContact] = useState<ContactBook>();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [failure, setFailure] = useState("");
  const [expired, setExpired] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    void readCampusContactBook(controller.signal).then(({ contact: result }) => {
      if (!controller.signal.aborted) setContact(result);
    }).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      setExpired(error instanceof PortalUnauthorizedError);
      setFailure(formatPortalError(error));
    });
    return () => controller.abort();
  }, [attempt]);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!contact || busy) return;
    setBusy(true);
    setFailure("");
    setMessage("");
    try {
      const response = await saveCampusContactBook(contact);
      setContact(response.contact);
      onSaved?.(response.contact);
      setMessage("联系方式已保存。");
    } catch (error) {
      setExpired(error instanceof PortalUnauthorizedError);
      setFailure(formatPortalError(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mt-4 space-y-3">
      {failure && <p role="alert" className="text-sm text-accent-text">{failure}</p>}
      {expired ? <Button asChild><Link href={loginForCampus(path)}>重新登录</Link></Button> : contact ? (
        <form onSubmit={(event) => void save(event)}>
          <fieldset disabled={busy} className="space-y-3">
            {(Object.keys(CONTACT_CHANNELS) as ContactChannel[]).map((channel) => (
              <label key={channel} className="block text-sm">
                {CONTACT_CHANNELS[channel]}
                <Input inputMode={channel === "phone" ? "tel" : undefined} maxLength={channel === "phone" ? 32 : 64} value={contact[channel]} onChange={(event) => setContact({ ...contact, [channel]: event.target.value })} />
              </label>
            ))}
            <label className="block text-sm">备注（选填）<Textarea maxLength={200} value={contact.note} onChange={(event) => setContact({ ...contact, note: event.target.value })} /></label>
            <Button type="submit">{busy ? "正在保存…" : "保存联系方式"}</Button>
          </fieldset>
        </form>
      ) : failure ? <Button onClick={() => { setFailure(""); setAttempt((value) => value + 1); }}>重新加载</Button> : <p role="status" className="text-sm">正在加载联系方式…</p>}
      {message && <p role="status" className="text-sm">{message}</p>}
    </div>
  );
}

export function CampusPublishDialog({ categories }: { categories: { key: string; name: string }[] }) {
  const { user, ready } = useCampusViewer();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  const [expired, setExpired] = useState(false);
  async function publish(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    const fields = new FormData(event.currentTarget);
    const value = (name: string) => String(fields.get(name) ?? "").trim();
    setBusy(true);
    setFailure("");
    try {
      const { item } = await publishCampusItem({ type: value("type") === "sell" ? "sell" : "help", category: value("category"), title: value("title"), desc: value("desc"), price: Number(value("price")), place: value("place"), deadline: value("deadline") || undefined });
      setOpen(false);
      router.push(`/campus/item/${encodeURIComponent(item.id)}`);
    } catch (error) {
      setExpired(error instanceof PortalUnauthorizedError);
      setFailure(formatPortalError(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog open={open} onOpenChange={(next) => { if (!busy) setOpen(next); }}>
      <DialogTrigger asChild><Button>发布新单子</Button></DialogTrigger>
      <DialogContent>
        <DialogTitle>发布新单子</DialogTitle>
        <DialogDescription>请勿在标题、地点或说明中填写私人联系方式。</DialogDescription>
        {ready && user ? (
          <form className="mt-4" onSubmit={(event) => void publish(event)}>
            <fieldset disabled={busy || expired} className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <label className="grid gap-1 text-sm">类型<Select name="type"><option value="help">互助</option><option value="sell">闲置</option></Select></label>
                <label className="grid gap-1 text-sm">分类<Select name="category" required>{categories.map(({ key, name }) => <option key={key} value={key}>{name}</option>)}</Select></label>
              </div>
              <label className="block text-sm">标题<Input name="title" maxLength={120} required /></label>
              <label className="block text-sm">说明<Textarea name="desc" maxLength={1000} required /></label>
              <label className="block text-sm">地点<Input name="place" maxLength={120} required /></label>
              <label className="block text-sm">金额（元）<Input name="price" type="number" min="0" max="100000" step="0.01" defaultValue="0" required /></label>
              <label className="block text-sm">时限（选填）<Input name="deadline" maxLength={120} /></label>
              <Button type="submit">{busy ? "正在发布…" : "确认发布"}</Button>
            </fieldset>
            {failure && <p role="alert" className="mt-3 text-sm text-accent-text">{failure}</p>}
            {expired && <Button asChild className="mt-3"><Link href={loginForCampus("/campus")}>重新登录</Link></Button>}
          </form>
        ) : ready ? <Button asChild className="mt-4"><Link href={loginForCampus("/campus")}>登录后发布</Link></Button> : <p role="status" className="mt-4 text-sm">正在核对登录状态…</p>}
      </DialogContent>
    </Dialog>
  );
}
