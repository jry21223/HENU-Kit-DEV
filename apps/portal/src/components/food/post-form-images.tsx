import type { ChangeEvent } from "react";
import Img from "@/components/ui/img";

export const MAX_IMAGES = 6;

export interface ImageDraft {
  file?: File;
  position?: number;
  preview: string;
}

export default function PostFormImages({ images, addImage, removeImage }: {
  images: ImageDraft[];
  addImage: (event: ChangeEvent<HTMLInputElement>) => Promise<void>;
  removeImage: (index: number) => void;
}) {
  return (
    <section className="border-b border-line py-6 md:py-8">
      <p className="font-mono text-xs text-ink/60">
        图片（{images.length}/{MAX_IMAGES}，单张 ≤2MB，可选）
      </p>
      <div className="mt-4 flex flex-wrap items-start gap-3">
        {images.map((image, index) => (
          <div key={index} className="flex w-28 flex-col">
            <Img
              src={image.preview}
              alt={`图 ${index + 1}`}
              label={`FIG.${index + 1}`}
              className="h-20 w-28"
            />
            {/* 删除是缩略图下方整行 44px 高的文字按钮，不压在图上（DESIGN_SYSTEM §13，#557）。 */}
            <button
              type="button"
              onClick={() => removeImage(index)}
              className="flex min-h-11 w-full items-center justify-center font-mono text-xs text-ink/60 transition-colors hover:text-accent-text"
              aria-label={`删除图 ${index + 1}`}
            >
              删除
            </button>
          </div>
        ))}
        {images.length < MAX_IMAGES && (
          <label className="flex h-20 w-28 cursor-pointer items-center justify-center border border-dashed border-ink/30 font-mono text-xs text-ink/60 transition-colors hover:border-ink hover:text-ink">
            + 上传
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp"
              className="hidden"
              onChange={(event) => void addImage(event)}
            />
          </label>
        )}
      </div>
    </section>
  );
}
