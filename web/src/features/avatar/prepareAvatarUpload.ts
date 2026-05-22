/** 上传前将图片居中裁剪为正方形并缩放到目标边长，避免小图被 QAvatar 放大发糊。 */
export async function prepareAvatarUploadFile(file: File, maxEdge = 512): Promise<File> {
  const mime = (file.type || "").toLowerCase();
  if (!["image/png", "image/jpeg", "image/webp"].includes(mime)) {
    return file;
  }
  const bitmap = await createImageBitmap(file);
  try {
    const side = Math.min(bitmap.width, bitmap.height);
    if (side < 64) {
      throw new Error("图片边长至少 64px，建议使用 256px 以上的正方形图片");
    }
    const sx = Math.floor((bitmap.width - side) / 2);
    const sy = Math.floor((bitmap.height - side) / 2);
    const outEdge = Math.min(maxEdge, side);
    const canvas = document.createElement("canvas");
    canvas.width = outEdge;
    canvas.height = outEdge;
    const ctx = canvas.getContext("2d");
    if (!ctx) return file;
    ctx.imageSmoothingEnabled = true;
    ctx.imageSmoothingQuality = "high";
    ctx.drawImage(bitmap, sx, sy, side, side, 0, 0, outEdge, outEdge);
    const outMime = mime === "image/png" ? "image/png" : "image/jpeg";
    const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, outMime, outMime === "image/jpeg" ? 0.92 : undefined));
    if (!blob) return file;
    const ext = outMime === "image/png" ? "png" : "jpg";
    const base = file.name.replace(/\.[^.]+$/, "") || "avatar";
    return new File([blob], `${base}.${ext}`, { type: outMime, lastModified: Date.now() });
  } finally {
    bitmap.close();
  }
}
