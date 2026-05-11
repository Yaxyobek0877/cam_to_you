/**
 * Wails API client — Go backend bilan ulanish nuqtasi.
 *
 * Wails build vaqtida `frontend/wailsjs/go/main/App.ts` yaratadi.
 * Wails Go enum'larini oddiy string'ga tarjima qiladi (literal union emas),
 * shu sababli har bir wrapper'da biz aniq UI tiplarimizga cast qilamiz.
 */

import type {
  Camera,
  Stream,
  StreamStatus,
  ProbeResult,
  FFmpegStatus,
} from "./types";

import * as App from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";

// `App` modulini "any" deb belgilab, har bir chaqirig'ga cast ishlatamiz.
// Bu Wails-generated bo'sh string tiplari va bizning literal union'lar
// orasidagi nomuvofiqlikni hal qiladi.
const A = App as any;

// FFmpeg
export const getFFmpegStatus = (): Promise<FFmpegStatus> => A.GetFFmpegStatus();
export const installFFmpeg = (): Promise<void> => A.InstallFFmpeg();
export const browseFFmpegFile = (): Promise<void> => A.BrowseFFmpegFile();

// Cameralar
export const listCameras = (): Promise<Camera[]> => A.ListCameras();
export const getCamera = (id: number): Promise<Camera> => A.GetCamera(id);
export const createCamera = (c: Partial<Camera>): Promise<Camera> => A.CreateCamera(c);
export const updateCamera = (c: Camera): Promise<void> => A.UpdateCamera(c);
export const deleteCamera = (id: number): Promise<void> => A.DeleteCamera(id);
export const probeCamera = (c: Partial<Camera>): Promise<ProbeResult> => A.ProbeCamera(c);
export const probeCameraSaved = (id: number): Promise<ProbeResult> => A.ProbeCameraSaved(id);

// Streamlar
export const listStreams = (): Promise<Stream[]> => A.ListStreams();
export const getStream = (id: number): Promise<Stream> => A.GetStream(id);
export const createStream = (s: Partial<Stream>): Promise<Stream> => A.CreateStream(s);
export const updateStream = (s: Stream): Promise<void> => A.UpdateStream(s);
export const deleteStream = (id: number): Promise<void> => A.DeleteStream(id);
export const startStream = (id: number): Promise<void> => A.StartStream(id);
export const stopStream = (id: number): Promise<void> => A.StopStream(id);
export const getStreamStatus = (id: number): Promise<StreamStatus> => A.GetStreamStatus(id);
export const getAllStreamStatus = (): Promise<Record<number, StreamStatus>> => A.GetAllStreamStatus();

// Preview (jonli ko'rish)
export const startPreview = (cameraId: number): Promise<void> => A.StartPreview(cameraId);
export const stopPreview = (cameraId: number): Promise<void> => A.StopPreview(cameraId);
export const isPreviewActive = (cameraId: number): Promise<boolean> => A.IsPreviewActive(cameraId);

// Window
export const hideWindow = (): Promise<void> => A.HideWindow();
export const quitApp = (): Promise<void> => A.Quit();

// Events — Go'dan keladigan hodisalar
export const onStreamEvent = (callback: (data: unknown) => void) => {
  return EventsOn("stream:event", callback);
};

export const onFFmpegProgress = (callback: (data: unknown) => void) => {
  return EventsOn("ffmpeg:progress", callback);
};
