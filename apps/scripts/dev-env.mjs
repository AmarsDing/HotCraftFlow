/** FrameWork local dev defaults: admin HTTP :7788 */
export function resolveApiTarget() {
  return process.env.TWIN_API_TARGET || process.env.FW_API_TARGET || 'http://127.0.0.1:7788';
}
export function resolveMapTarget() {
  return process.env.TWIN_MAP_TARGET || process.env.FW_MAP_TARGET || 'http://127.0.0.1:3000';
}
export function describeDevEnv() {
  return `api=${resolveApiTarget()} map=${resolveMapTarget()}`;
}
