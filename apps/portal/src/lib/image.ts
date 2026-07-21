/**
 * 图片工具：上传转 dataURL（事件回调内使用）+ picsum 确定性 seed 外链。
 * v1 无服务端存储：上传图仅存会话内存，刷新还原。
 */

export const IMAGE_ACCEPT = ["image/jpeg", "image/png", "image/webp"];
export const IMAGE_MAX_SIZE = 2 * 1024 * 1024; // 2MB

export function fileToDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    if (!IMAGE_ACCEPT.includes(file.type)) {
      reject(new Error("仅支持 JPG / PNG / WebP 图片"));
      return;
    }
    if (file.size > IMAGE_MAX_SIZE) {
      reject(new Error("图片大小不能超过 2MB"));
      return;
    }
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(new Error("图片读取失败"));
    reader.readAsDataURL(file);
  });
}

/** picsum 确定性 seed 图（有网即真图，失败由 Img 组件回退占位块） */
export function seedImg(seed: string, w: number, h: number) {
  return `https://picsum.photos/seed/${seed}/${w}/${h}`;
}
