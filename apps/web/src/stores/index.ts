import { createPinia } from 'pinia';
import { store } from 'quasar/wrappers';

export default store(() => createPinia());

export { useAppStore } from './app';
export { useAuthStore } from './auth';
export { useSystemNameStore } from './system-name';
export { useMyNoticeStore } from './my-notice';
export {
  useUserStore,
  useRoleStore,
  useMenuStore,
  useOrganizationStore,
  useDepartmentStore,
  usePositionStore,
  useDictStore,
  useTenantStore,
  useToolLinkStore,
  useToolLinkCategoryStore,
  useTaskStore,
  useApiResourceStore,
  useLoginLogStore,
  useOperationLogStore,
  useLoginRestrictionStore,
  useNotificationStore,
  usePrivateMessageStore,
  useFileStore,
  useSystemConfigStore,
  useDataPermissionStore,
  buildMenuTree,
  buildTree,
  type TreeNode,
} from './modules';
