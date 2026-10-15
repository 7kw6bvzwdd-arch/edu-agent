// Authenticated fetch streams use the same session renewal and error handling as
// other API calls. Do not reconnect POSTs automatically: they can cost model use.
export async function readEventStream(response, onEvent) {
  if (!response.headers.get('content-type')?.startsWith('text/event-stream')) {
    const value = await response.json()
    onEvent('done', value)
    return value
  }
  const reader = response.body.getReader(), decoder = new TextDecoder()
  let buffer = '', result, completed = false
  try {
    while (true) {
      const {value, done} = await reader.read()
      buffer += decoder.decode(value, {stream: !done}).replace(/\r\n/g, '\n')
      let end
      while ((end = buffer.indexOf('\n\n')) >= 0) {
        const lines = buffer.slice(0, end).split('\n')
        buffer = buffer.slice(end + 2)
        const event = lines.find(line => line.startsWith('event:'))?.slice(6).trim() || 'message'
        const data = lines.filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n')
        if (!data) continue
        const item = JSON.parse(data)
        if (event === 'error') throw Error(item.message || '对话中断，需求已保留，请重试。')
        onEvent(event, item)
        if (event === 'done') { result = item; completed = true }
      }
      if (done) break
    }
    if (!completed) throw Error('对话连接中断，已填写的需求保留，请重试。')
    return result
  } finally {
    await reader.cancel().catch(() => {})
    reader.releaseLock()
  }
}
