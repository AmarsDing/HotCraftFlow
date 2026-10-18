import fs from 'fs';
const p = process.argv[2];
if (!p) { console.error('need path'); process.exit(1); }
let mod = fs.readFileSync(p, 'utf8');
const drop = [
  'monitorAssetService','deviceCategoryService','deviceManufacturerService','deviceModelService',
  'alarmLevelService','alarmNoticeProfileService','alarmDeviceTemplateService',
  'type MonitorAssetEntity','type DeviceCategoryEntity','type DeviceManufacturerEntity','type DeviceModelEntity',
  'type AlarmLevelEntity','type AlarmNoticeProfileEntity','type AlarmDeviceTemplateEntity',
];
for (const n of drop) {
  const re = new RegExp('^\\s*' + n.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + ',?\\r?\\n', 'gm');
  mod = mod.replace(re, '');
}
mod = mod.replace(/\/\/ Asset module stores[\s\S]*?(?=\/\/ Generic tree builder)/, '');
mod = mod.replace(/\/\/ Alarm module stores[\s\S]*$/m, '');
mod = mod.replace(/^export const useMonitorAssetStore.*\r?\n/gm, '');
mod = mod.replace(/^export const useDeviceCategoryStore.*\r?\n/gm, '');
mod = mod.replace(/^export const useDeviceManufacturerStore[\s\S]*?;\r?\n/m, '');
mod = mod.replace(/^export const useDeviceModelStore.*\r?\n/gm, '');
mod = mod.replace(/^export const useAlarmLevelStore.*\r?\n/gm, '');
mod = mod.replace(/^export const useAlarmNoticeProfileStore[\s\S]*?;\r?\n/m, '');
mod = mod.replace(/^export const useAlarmDeviceTemplateStore[\s\S]*?;\r?\n/m, '');
fs.writeFileSync(p, mod);
console.log('slimmed bytes', Buffer.byteLength(mod,'utf8'));
console.log('has MonitorAsset', /MonitorAsset/.test(mod));
console.log('has AlarmLevel', /AlarmLevel/.test(mod));
