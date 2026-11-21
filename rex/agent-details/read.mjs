import { getSessionInfo, getSessionMessages } from "@anthropic-ai/claude-agent-sdk";

const [sessionId, cwd] = process.argv.slice(2);
if (!/^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(sessionId ?? "")) {
  process.exit(2);
}

const options = cwd ? { dir: cwd } : {};
const info = await getSessionInfo(sessionId, options);
if (info?.sessionId !== sessionId) {
  process.exit(1);
}

let result, model;
if (Number.isSafeInteger(info.fileSize) && info.fileSize <= 4 * 1024 * 1024) {
  const messages = await getSessionMessages(sessionId, options);
  for (let i = messages.length - 1; i >= 0; i--) {
    const entry = messages[i];
    if (entry.session_id !== sessionId || entry.type !== "assistant" || entry.parent_tool_use_id) continue;
    const content = entry.message?.content;
    if (!Array.isArray(content) || content.some((item) => item.type === "tool_use")) continue;
    const value = content.filter((item) => item.type === "text" && typeof item.text === "string")
      .map((item) => item.text).join(" ").trim();
    if (!value) continue;
    result = value.slice(0, 840);
    model = typeof entry.message?.model === "string" ? entry.message.model.slice(0, 80) : undefined;
    break;
  }
}
console.log(JSON.stringify({ session_id: sessionId, result, model, updated: info.lastModified }));
