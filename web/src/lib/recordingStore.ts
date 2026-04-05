const DB_NAME = "jerboa";
const STORE = "recordings";

export interface PendingRecording {
  id: string;
  bandSlug: string;
  songId?: string;
  overdubParentId?: string;
  offsetMs?: number;
  mimeType: string;
  filename: string;
  timestamp: number;
  chunks: Blob[];
}

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 1);
    req.onupgradeneeded = () => {
      req.result.createObjectStore(STORE, { keyPath: "id" });
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

export async function saveChunk(session: PendingRecording, chunk: Blob): Promise<void> {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, "readwrite");
    const store = tx.objectStore(STORE);
    const get = store.get(session.id);
    get.onsuccess = () => {
      const existing: PendingRecording = get.result ?? { ...session, chunks: [] };
      existing.chunks.push(chunk);
      store.put(existing);
    };
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}

export async function getPendingRecordings(bandSlug: string): Promise<PendingRecording[]> {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, "readonly");
    const req = tx.objectStore(STORE).getAll();
    req.onsuccess = () =>
      resolve((req.result as PendingRecording[]).filter((r) => r.bandSlug === bandSlug));
    req.onerror = () => reject(req.error);
  });
}

export async function clearRecording(id: string): Promise<void> {
  const db = await openDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, "readwrite");
    tx.objectStore(STORE).delete(id);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
}
