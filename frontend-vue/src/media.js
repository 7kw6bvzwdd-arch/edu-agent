export async function uploadTeachingImage(file, token) {
  if (!['image/png','image/jpeg','image/gif'].includes(file.type) || file.size > 8*1024*1024) throw Error('请选择8MB以内的PNG、JPEG或GIF图片。');
  const form = new FormData(); form.append('file', file);
  const response = await fetch('/api/media', {method:'POST', headers:{Authorization:'Bearer '+token}, body:form});
  const result = await response.json();
  if (!response.ok) throw Error(result.message || '图片上传失败，请重试。');
  return result;
}
