<template>
  <q-page class="profile-page" padding>
    <div class="profile-shell">
      <!-- 顶部 Hero：紧凑身份条（头像 + 身份 + 元数据 + 完整度环 + 登录信息） -->
      <q-card class="profile-hero">
        <div class="profile-hero__scanline" />
        <q-card-section class="profile-hero__inner">
          <div class="profile-hero__avatar-wrap" @click="openAvatarDialog">
            <UserAvatar :avatar="heroAvatar" :name="displayName" :size="64" class="profile-hero__avatar" />
            <span class="profile-hero__avatar-edit">
              <q-icon name="photo_camera" size="11px" />
            </span>
            <q-tooltip>{{ t('profile.editAvatar') }}</q-tooltip>
          </div>

          <div class="profile-hero__info">
            <div class="profile-hero__name-row">
              <span class="profile-hero__name">{{ displayName }}</span>
              <span class="profile-hero__status-dot" :class="{ 'is-on': statusOn }" />
              <span class="profile-hero__status-text">
                {{ statusOn ? t('user.statusEnabled') : t('user.statusDisabled') }}
              </span>
              <span class="profile-hero__username">@{{ profile?.username || '-' }}</span>
              <template v-if="roleLabels.length">
                <q-chip v-for="role in roleLabels" :key="role" dense outline color="primary" size="sm">
                  {{ role }}
                </q-chip>
              </template>
            </div>
            <div class="profile-hero__meta">
              <div v-if="dutyLabel" class="profile-meta">
                <q-icon name="workspace_premium" size="14px" />
                <span class="profile-meta__value">{{ dutyLabel }}</span>
              </div>
              <div class="profile-meta">
                <q-icon name="apartment" size="14px" />
                <span class="profile-meta__value">{{ departmentName || '-' }}</span>
              </div>
              <div class="profile-meta">
                <q-icon name="badge" size="14px" />
                <span class="profile-meta__value">{{ positionName || '-' }}</span>
              </div>
              <div class="profile-meta">
                <q-icon name="pin" size="14px" />
                <span class="profile-meta__value">{{ profile?.workId || '-' }}</span>
              </div>
            </div>
          </div>

          <div class="profile-hero__side">
            <div class="profile-hero__ring">
              <q-circular-progress
                :value="completeness"
                show-value
                size="48px"
                :thickness="0.18"
                color="primary"
                :track-color="$q.dark.isActive ? 'blue-grey-9' : 'grey-3'"
                class="profile-hero__progress"
              />
              <span class="profile-hero__ring-label">{{ t('profile.completeness') }}</span>
            </div>
            <div class="profile-hero__login">
              <div class="profile-login-item">
                <q-icon name="schedule" size="14px" class="profile-login-item__icon" />
                <span class="profile-login-item__label">{{ t('user.lastLoginTime') }}</span>
                <span class="profile-login-item__value">{{ lastLoginText }}</span>
              </div>
              <div class="profile-login-item">
                <q-icon name="public" size="14px" class="profile-login-item__icon" />
                <span class="profile-login-item__label">{{ t('profile.lastLoginIp') }}</span>
                <span class="profile-login-item__value">{{ profile?.lastLoginIp || '-' }}</span>
              </div>
            </div>
          </div>
        </q-card-section>
      </q-card>

      <div class="row q-col-gutter-md">
        <!-- 基本信息 -->
        <div class="col-12 col-lg-7">
          <q-card class="profile-card profile-card--fill">
            <q-card-section class="profile-card__header">
              <span class="profile-card__icon"><q-icon name="person" size="16px" /></span>
              <span class="profile-card__title">{{ t('profile.basicInfo') }}</span>
              <q-space />
              <q-btn
                unelevated
                dense
                color="primary"
                icon="save"
                :loading="savingBasic"
                :label="t('common.save')"
                @click="onSaveBasic"
              />
            </q-card-section>
            <q-separator />
            <q-card-section class="profile-form">
              <div class="row q-col-gutter-sm">
                <div class="col-12 col-sm-6">
                  <q-input v-model="basicForm.nickname" :label="t('user.nickname')" dense outlined hide-bottom-space>
                    <template #prepend><q-icon name="person_outline" size="16px" /></template>
                  </q-input>
                </div>
                <div class="col-12 col-sm-6">
                  <q-input v-model="basicForm.realname" :label="t('user.realName')" dense outlined hide-bottom-space>
                    <template #prepend><q-icon name="perm_identity" size="16px" /></template>
                  </q-input>
                </div>
                <div class="col-12 col-sm-6">
                  <q-select
                    v-model="basicForm.gender"
                    :options="genderOptions"
                    :label="t('user.gender')"
                    emit-value
                    map-options
                    dense
                    outlined
                    hide-bottom-space
                  >
                    <template #prepend><q-icon name="wc" size="16px" /></template>
                  </q-select>
                </div>
                <div class="col-12 col-sm-6">
                  <q-input v-model="basicForm.region" :label="t('user.region')" dense outlined hide-bottom-space>
                    <template #prepend><q-icon name="public" size="16px" /></template>
                  </q-input>
                </div>
                <div class="col-12 col-sm-6">
                  <q-input v-model="basicForm.email" :label="t('user.email')" dense outlined hide-bottom-space>
                    <template #prepend><q-icon name="mail_outline" size="16px" /></template>
                  </q-input>
                </div>
                <div class="col-12 col-sm-6">
                  <q-input v-model="basicForm.mobile" :label="t('user.mobile')" dense outlined hide-bottom-space>
                    <template #prepend><q-icon name="smartphone" size="16px" /></template>
                  </q-input>
                </div>
                <div class="col-12 col-sm-6">
                  <q-input v-model="basicForm.telephone" :label="t('user.telephone')" dense outlined hide-bottom-space>
                    <template #prepend><q-icon name="call" size="16px" /></template>
                  </q-input>
                </div>
                <div class="col-12 col-sm-6">
                  <q-input v-model="basicForm.address" :label="t('user.address')" dense outlined hide-bottom-space>
                    <template #prepend><q-icon name="place" size="16px" /></template>
                  </q-input>
                </div>
                <div class="col-12">
                  <q-input
                    v-model="basicForm.description"
                    :label="t('profile.description')"
                    type="textarea"
                    rows="2"
                    autogrow
                    dense
                    outlined
                    hide-bottom-space
                  />
                </div>
              </div>
            </q-card-section>
          </q-card>
        </div>

        <!-- 账号安全（登录密码 / 操作密码 / MFA 合并列表） -->
        <div class="col-12 col-lg-5">
          <q-card class="profile-card profile-card--fill">
            <q-card-section class="profile-card__header">
              <span class="profile-card__icon"><q-icon name="shield" size="16px" /></span>
              <span class="profile-card__title">{{ t('profile.securitySection') }}</span>
            </q-card-section>
            <q-separator />

            <!-- 登录密码 -->
            <div class="sec-item">
              <span class="sec-item__icon"><q-icon name="lock_reset" size="15px" /></span>
              <div class="sec-item__body">
                <div class="sec-item__title">{{ t('profile.loginPassword') }}</div>
                <div class="sec-item__hint">{{ t('profile.loginPasswordHint') }}</div>
                <q-slide-transition>
                  <div v-show="pwdExpanded" class="sec-item__form">
                    <div class="row q-col-gutter-sm">
                      <div class="col-12 col-sm-4">
                        <q-input
                          v-model="pwdForm.oldPassword"
                          :label="t('common.oldPassword')"
                          dense
                          outlined
                          hide-bottom-space
                          type="password"
                        />
                      </div>
                      <div class="col-12 col-sm-4">
                        <q-input
                          v-model="pwdForm.newPassword"
                          :label="t('common.newPassword')"
                          dense
                          outlined
                          hide-bottom-space
                          type="password"
                        />
                      </div>
                      <div class="col-12 col-sm-4">
                        <q-input
                          v-model="pwdForm.confirmPassword"
                          :label="t('common.confirmPassword')"
                          dense
                          outlined
                          hide-bottom-space
                          type="password"
                        />
                      </div>
                    </div>
                    <div class="sec-item__form-actions">
                      <q-btn flat dense :label="t('common.cancel')" @click="pwdExpanded = false" />
                      <q-btn
                        unelevated
                        dense
                        color="primary"
                        :loading="savingPwd"
                        :label="t('common.confirm')"
                        @click="onChangePassword"
                      />
                    </div>
                  </div>
                </q-slide-transition>
              </div>
              <q-btn
                v-if="!pwdExpanded"
                outline
                dense
                color="primary"
                class="sec-item__action"
                :label="t('common.edit')"
                @click="pwdExpanded = true"
              />
              <q-btn v-else flat dense round icon="expand_less" class="sec-item__action" @click="pwdExpanded = false" />
            </div>
            <q-separator />

            <!-- 操作密码（凭证管理二次鉴权） -->
            <div class="sec-item">
              <span class="sec-item__icon"><q-icon name="key" size="15px" /></span>
              <div class="sec-item__body">
                <div class="sec-item__title">
                  {{ t('profile.opPwdSection') }}
                  <q-badge :color="opPwdSet ? 'positive' : 'grey'" class="sec-item__badge">
                    {{ opPwdSet ? t('profile.opPwdStatusSet') : t('profile.opPwdStatusUnset') }}
                  </q-badge>
                </div>
                <div class="sec-item__hint">{{ t('profile.opPwdHint') }}</div>
              </div>
              <q-btn
                outline
                dense
                color="primary"
                class="sec-item__action"
                :label="opPwdSet ? t('profile.changeOpPwd') : t('profile.setOpPwd')"
                @click="openOpPwdDialog"
              />
            </div>
            <q-separator />

            <!-- MFA 多因素认证 -->
            <div class="sec-item">
              <span class="sec-item__icon"><q-icon name="verified_user" size="15px" /></span>
              <div class="sec-item__body">
                <div class="sec-item__title">
                  {{ t('profile.mfaSection') }}
                  <q-badge v-if="!mfaDetecting" :color="mfaEnabled ? 'positive' : 'grey'" class="sec-item__badge">
                    {{ mfaEnabled ? t('profile.mfaEnabled') : t('profile.mfaDisabled') }}
                  </q-badge>
                </div>
                <div class="sec-item__hint">{{ t('profile.mfaHint') }}</div>
              </div>
              <template v-if="mfaDetecting">
                <q-spinner size="sm" class="sec-item__action" />
              </template>
              <q-btn
                v-else-if="!mfaEnabled"
                outline
                dense
                color="primary"
                class="sec-item__action"
                :loading="enablingMFA"
                :label="t('profile.enableMfa')"
                @click="onEnableMFA"
              />
              <q-btn
                v-else
                outline
                dense
                color="negative"
                class="sec-item__action"
                :label="t('profile.disableMfa')"
                @click="disableDialog = true"
              />
            </div>
          </q-card>
        </div>
      </div>
    </div>

    <!-- 更换头像对话框（类微信：大图预览 + 更换/移除 + 拖拽/粘贴 + 裁剪上传） -->
    <AvatarChangeDialog
      v-model="avatarDialog"
      :avatar="heroAvatar"
      :name="displayName"
      :username="profile?.username"
    />

    <!-- 设置 / 修改操作密码对话框 -->
    <q-dialog v-model="opPwdDialog" persistent>
      <q-card style="min-width: 360px">
        <q-card-section>
          <div class="text-h6">
            {{ opPwdSet ? t('profile.changeOpPwd') : t('profile.setOpPwd') }}
          </div>
        </q-card-section>
        <q-card-section class="q-gutter-md">
          <q-input
            v-model="opPwdForm.oldPassword"
            :placeholder="t('profile.oldOpPwd')"
            :hint="t('profile.oldOpPwdHint')"
            dense
            outlined
            type="password"
            maxlength="6"
          />
          <q-input
            v-model="opPwdForm.newPassword"
            :placeholder="t('profile.newOpPwd')"
            dense
            outlined
            type="password"
            maxlength="6"
            :rules="[(val) => /^\d{6}$/.test(val) || t('profile.opPwdInvalid')]"
          />
          <q-input
            v-model="opPwdForm.confirmPassword"
            :placeholder="t('profile.confirmOpPwd')"
            dense
            outlined
            type="password"
            maxlength="6"
            :rules="[(val) => val === opPwdForm.newPassword || t('profile.opPwdMismatch')]"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat :label="t('common.cancel')" />
          <q-btn
            unelevated
            color="primary"
            :loading="savingOpPwd"
            :label="t('common.confirm')"
            @click="onSubmitOpPwd"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 启用 MFA 对话框：显示 QR + secret（后端仅落密钥未启用），输入首个验证码调 ConfirmMFA 正式启用 -->
    <q-dialog v-model="enableDialog" persistent>
      <q-card style="min-width: 360px">
        <q-card-section>
          <div class="text-h6">{{ t('profile.enableMfa') }}</div>
        </q-card-section>
        <q-card-section class="column items-center q-gutter-md">
          <div>{{ t('profile.scanQr') }}</div>
          <canvas ref="qrCanvas" />
          <div class="row items-center q-gutter-sm">
            <span>{{ t('profile.secret') }}:</span>
            <code>{{ mfaSetupData?.secret }}</code>
          </div>
          <q-input
            v-model="confirmMfaCode"
            :placeholder="t('profile.mfaCode')"
            dense
            outlined
            maxlength="6"
            style="width: 200px"
            :rules="[(val) => val.length === 6 || t('common.required')]"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat :label="t('common.cancel')" @click="onCancelEnableMFA" />
          <q-btn
            unelevated
            color="primary"
            :loading="confirmingMFA"
            :disable="confirmMfaCode.length !== 6"
            :label="t('common.confirm')"
            @click="onConfirmEnableMFA"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 禁用 MFA 对话框 -->
    <q-dialog v-model="disableDialog">
      <q-card style="min-width: 320px">
        <q-card-section>
          <div class="text-h6">{{ t('profile.disableMfa') }}</div>
        </q-card-section>
        <q-card-section>
          <q-input
            v-model="disableMfaCode"
            :placeholder="t('profile.mfaCode')"
            dense
            outlined
            maxlength="6"
            :rules="[(val) => val.length === 6 || t('common.required')]"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat :label="t('common.cancel')" />
          <q-btn
            unelevated
            color="negative"
            :loading="disablingMFA"
            :label="t('common.confirm')"
            @click="onConfirmDisableMFA"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify, date, useQuasar } from 'quasar';
import QRCode from 'qrcode';
import { useAuthStore, useDepartmentStore, usePositionStore, useRoleStore } from '../../stores';
import { authService, type UserProfile } from '../../services';
import UserAvatar from '../../components/UserAvatar.vue';
import AvatarChangeDialog from '../../components/AvatarChangeDialog.vue';

const { t } = useI18n();
const $q = useQuasar();
const auth = useAuthStore();
const departmentStore = useDepartmentStore();
const positionStore = usePositionStore();
const roleStore = useRoleStore();

const profile = ref<UserProfile | null>(null);

const basicForm = ref({
  nickname: '',
  realname: '',
  email: '',
  mobile: '',
  telephone: '',
  gender: '' as string,
  region: '',
  address: '',
  description: '',
});
const savingBasic = ref(false);

const genderOptions = computed(() => [
  { label: t('user.genderMale'), value: 'MALE' },
  { label: t('user.genderFemale'), value: 'FEMALE' },
  { label: t('user.genderUnknown'), value: 'SECRET' },
]);

const pwdForm = ref({
  oldPassword: '',
  newPassword: '',
  confirmPassword: '',
});
const savingPwd = ref(false);
// 登录密码修改默认折叠，点击“编辑”内联展开（节省安全卡垂直空间）
const pwdExpanded = ref(false);

// 资料完整度：9 个可编辑字段的填写占比，Hero 环与表单实时联动
const completeness = computed(() => {
  const f = basicForm.value;
  const fields = [f.nickname, f.realname, f.gender, f.region, f.email, f.mobile, f.telephone, f.address, f.description];
  const filled = fields.filter((v) => String(v ?? '').trim().length > 0).length;
  return Math.round((filled / fields.length) * 100);
});

// 头像（类微信弹窗：预览/更换/移除/拖拽/粘贴，内部完成上传与顶栏同步）
const avatarDialog = ref(false);

// 操作密码（§2.6.6 凭证明文查看等敏感操作二次鉴权）
const opPwdDialog = ref(false);
const opPwdSet = ref(false);
const opPwdForm = ref({ oldPassword: '', newPassword: '', confirmPassword: '' });
const savingOpPwd = ref(false);

// MFA
const mfaDetecting = ref(false);
const mfaEnabled = ref(false);
const enableDialog = ref(false);
const disableDialog = ref(false);
const mfaSetupData = ref<{ secret: string; otpauthUrl: string } | null>(null);
const disableMfaCode = ref('');
const confirmMfaCode = ref('');
const enablingMFA = ref(false);
const disablingMFA = ref(false);
const confirmingMFA = ref(false);
const qrCanvas = ref<HTMLCanvasElement | null>(null);

const displayName = computed(() => profile.value?.nickname || profile.value?.username || '');
// 头像以 auth store 为单一数据源：本页弹窗与顶栏弹窗均经 refreshProfile 刷新 store，
// Hero/弹窗直读 store，避免本地 profile 副本在跨入口变更后失同步
const heroAvatar = computed(() => auth.profile?.avatar || '');
const statusOn = computed(() => (profile.value?.status ?? 'ON') === 'ON');

// 职务：展示后端 authority（权限级别）映射
const dutyLabel = computed(() => {
  switch (profile.value?.authority) {
    case 'SYS_ADMIN':
      return t('user.authoritySysAdmin');
    case 'TENANT_ADMIN':
      return t('user.authorityTenantAdmin');
    case 'CUSTOMER_USER':
      return t('user.authorityCustomer');
    case 'GUEST':
      return t('user.authorityGuest');
    default:
      return '';
  }
});

const departmentName = computed(() => {
  const id = Number(profile.value?.orgId);
  if (!id) return '';
  return departmentStore.list.find((d) => Number(d.id) === id)?.name || '';
});

const positionName = computed(() => {
  const id = Number(profile.value?.positionId);
  if (!id) return '';
  return positionStore.list.find((p) => Number(p.id) === id)?.name || '';
});

const roleLabels = computed(() => {
  const codes = profile.value?.roles ?? [];
  return codes.map((code) => roleStore.list.find((r) => r.code === code)?.name || code);
});

const lastLoginText = computed(() => {
  const v = profile.value?.lastLoginTime;
  if (!v) return '-';
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? '-' : date.formatDate(d, 'YYYY-MM-DD HH:mm');
});

function opPwdStorageKey() {
  return `framework_op_pwd_set_${profile.value?.id ?? 'anon'}`;
}

async function loadProfile() {
  try {
    const data = await authService.getMe();
    profile.value = data;
    basicForm.value = {
      nickname: data.nickname || '',
      realname: data.realname || '',
      email: data.email || '',
      mobile: data.mobile || '',
      telephone: data.telephone || '',
      // 兼容历史脏数据：后端枚举为 SECRET/MALE/FEMALE
      gender: data.gender === 'UNKNOWN' ? 'SECRET' : data.gender || '',
      region: data.region || '',
      address: data.address || '',
      description: data.description || '',
    };
    opPwdSet.value = localStorage.getItem(opPwdStorageKey()) === '1';
    // MFA 启用状态回填（B7：User DTO 下发 mfaEnabled，替代历史"无法探测"注释口径）
    mfaEnabled.value = data.mfaEnabled ?? false;
  } catch {
    // error notification handled by axios interceptor
  }
}

async function onSaveBasic() {
  savingBasic.value = true;
  try {
    await authService.updateMe({
      nickname: basicForm.value.nickname,
      realname: basicForm.value.realname,
      email: basicForm.value.email,
      mobile: basicForm.value.mobile,
      telephone: basicForm.value.telephone,
      gender: basicForm.value.gender || undefined,
      region: basicForm.value.region,
      address: basicForm.value.address,
      description: basicForm.value.description,
    });
    await auth.refreshProfile();
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
  } catch {
    // error notification handled by axios interceptor
  } finally {
    savingBasic.value = false;
  }
}

async function onChangePassword() {
  if (!pwdForm.value.oldPassword || !pwdForm.value.newPassword) {
    Notify.create({ type: 'negative', message: t('common.required') });
    return;
  }
  if (pwdForm.value.newPassword !== pwdForm.value.confirmPassword) {
    Notify.create({ type: 'negative', message: t('profile.pwdMismatch') });
    return;
  }
  savingPwd.value = true;
  try {
    await authService.changePassword({
      oldPassword: pwdForm.value.oldPassword,
      newPassword: pwdForm.value.newPassword,
    });
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
    pwdForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' };
    pwdExpanded.value = false;
  } catch {
    // error notification handled by axios interceptor
  } finally {
    savingPwd.value = false;
  }
}

function openAvatarDialog() {
  avatarDialog.value = true;
}

function openOpPwdDialog() {
  opPwdForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' };
  opPwdDialog.value = true;
}

async function onSubmitOpPwd() {
  const { oldPassword, newPassword, confirmPassword } = opPwdForm.value;
  if (!/^\d{6}$/.test(newPassword)) {
    Notify.create({ type: 'negative', message: t('profile.opPwdInvalid') });
    return;
  }
  if (newPassword !== confirmPassword) {
    Notify.create({ type: 'negative', message: t('profile.opPwdMismatch') });
    return;
  }
  savingOpPwd.value = true;
  try {
    await authService.setOperationPassword({
      operationPassword: newPassword,
      oldOperationPassword: oldPassword || undefined,
    });
    opPwdSet.value = true;
    localStorage.setItem(opPwdStorageKey(), '1');
    opPwdDialog.value = false;
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
  } catch (e) {
    // 后端要求旧操作密码时标记为"已设置"，下次提交强制带旧密码
    const msg = (e as { response?: { data?: { message?: string } } })?.response?.data?.message || '';
    if (msg.includes('old operation password')) {
      opPwdSet.value = true;
      localStorage.setItem(opPwdStorageKey(), '1');
    }
  } finally {
    savingOpPwd.value = false;
  }
}

// MFA 启用状态由后端 User DTO 的 mfaEnabled 字段下发（B7 契约补齐），loadProfile 直接回填。

async function onEnableMFA() {
  enablingMFA.value = true;
  try {
    // 后端 EnableMFA 仅生成并落密钥（mfa_enabled=false 未启用），返回 secret + otpauthUrl 供扫码；
    // 用户在对话框输入首个验证码后调 ConfirmMFA 才正式启用（防扫错码锁死账户）
    const resp = await authService.enableMFA();
    mfaSetupData.value = resp;
    confirmMfaCode.value = '';
    enableDialog.value = true;
  } catch {
    // error notification handled by axios interceptor
  } finally {
    enablingMFA.value = false;
  }
}

async function renderQR() {
  await nextTick();
  if (qrCanvas.value && mfaSetupData.value?.otpauthUrl) {
    await QRCode.toCanvas(qrCanvas.value, mfaSetupData.value.otpauthUrl, { width: 240 });
  }
}

watch(enableDialog, (val) => {
  if (val) renderQR();
});

function onCancelEnableMFA() {
  enableDialog.value = false;
}

async function onConfirmEnableMFA() {
  // 校验首个 TOTP 验证码，通过后后端才置 mfa_enabled=true
  confirmingMFA.value = true;
  try {
    await authService.confirmMFA(confirmMfaCode.value);
    mfaEnabled.value = true;
    mfaSetupData.value = null;
    confirmMfaCode.value = '';
    enableDialog.value = false;
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
  } catch {
    // error notification handled by axios interceptor（验证码错误等对话框保持打开可重试）
  } finally {
    confirmingMFA.value = false;
  }
}

async function onConfirmDisableMFA() {
  disablingMFA.value = true;
  try {
    await auth.disableMFA(disableMfaCode.value);
    mfaEnabled.value = false;
    disableDialog.value = false;
    disableMfaCode.value = '';
    Notify.create({ type: 'positive', message: t('common.updateSuccess') });
  } catch {
    // error notification handled by axios interceptor
  } finally {
    disablingMFA.value = false;
  }
}

onMounted(() => {
  loadProfile();
  // 部门 / 岗位 / 角色名称映射（失败静默，Hero 区回退显示 '-' / 原角色码）
  if (!departmentStore.list.length) void departmentStore.fetchList({ noPaging: true }).catch(() => {});
  if (!positionStore.list.length) void positionStore.fetchList({ noPaging: true }).catch(() => {});
  if (!roleStore.list.length) void roleStore.fetchList({ noPaging: true }).catch(() => {});
});
</script>

<style lang="sass" scoped>
.profile-page
  background: var(--gradient-page)

.profile-shell
  max-width: 1200px
  margin: 0 auto
  display: flex
  flex-direction: column
  gap: var(--space-3)

// ── Hero 紧凑身份条 ──
.profile-hero
  position: relative
  border-radius: 14px
  overflow: hidden

// 顶部科技感渐变扫描线
.profile-hero__scanline
  position: absolute
  top: 0
  left: 0
  right: 0
  height: 2px
  background: linear-gradient(90deg, transparent 0%, var(--color-accent) 30%, var(--color-accent) 70%, transparent 100%)
  opacity: 0.55
  pointer-events: none

.profile-hero__inner
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: var(--space-4)
  padding: var(--space-3) var(--space-4)

.profile-hero__avatar-wrap
  position: relative
  flex-shrink: 0
  cursor: pointer
  border-radius: 50%
  transition: box-shadow 0.18s ease
  &:hover
    box-shadow: var(--shadow-accent-focus)
    .profile-hero__avatar-edit
      transform: scale(1.08)

.profile-hero__avatar
  background: var(--color-accent)
  color: var(--color-on-accent)
  font-size: 24px
  font-weight: 700
  border: 2px solid color-mix(in srgb, var(--color-accent) 40%, var(--glass-border))
  img
    object-fit: cover

.profile-hero__avatar-edit
  position: absolute
  right: -2px
  bottom: -2px
  display: flex
  align-items: center
  justify-content: center
  width: 20px
  height: 20px
  border-radius: 50%
  background: var(--color-accent)
  color: var(--color-on-accent)
  border: 2px solid var(--canvas-base)
  transition: transform 0.15s ease

.profile-hero__info
  flex: 1 1 300px
  min-width: 0

.profile-hero__name-row
  display: flex
  align-items: center
  gap: 8px
  flex-wrap: wrap

.profile-hero__name
  font-size: 1.15rem
  font-weight: 700
  letter-spacing: -0.01em
  color: var(--color-text-heading)

// 状态呼吸点（替代原大 Badge，节省空间）
.profile-hero__status-dot
  width: 7px
  height: 7px
  border-radius: 50%
  background: var(--color-text-tertiary)
  &.is-on
    background: var(--color-success)
    box-shadow: 0 0 6px color-mix(in srgb, var(--color-success) 70%, transparent)

.profile-hero__status-text
  font-size: var(--text-xs)
  color: var(--color-text-secondary)

.profile-hero__username
  font-size: var(--text-xs)
  color: var(--color-text-tertiary)

.profile-hero__meta
  display: flex
  flex-wrap: wrap
  gap: 4px var(--space-4)
  margin-top: 6px

.profile-meta
  display: inline-flex
  align-items: center
  gap: 5px
  color: var(--color-accent)
  &__value
    font-size: var(--text-xs)
    font-weight: 600
    color: var(--color-text-secondary)

// 右侧：完整度环 + 登录信息
.profile-hero__side
  display: flex
  align-items: center
  gap: var(--space-4)
  flex: 0 0 auto
  margin-left: auto

.profile-hero__ring
  display: flex
  flex-direction: column
  align-items: center
  gap: 2px

.profile-hero__progress
  :deep(.q-circular-progress__content)
    font-size: 12px
    font-weight: 700
    color: var(--color-text-heading)
    font-variant-numeric: tabular-nums

.profile-hero__ring-label
  font-size: 10px
  color: var(--color-text-tertiary)
  white-space: nowrap

.profile-hero__login
  display: flex
  flex-direction: column
  gap: 4px
  padding-left: var(--space-4)
  border-left: 1px solid var(--glass-border)

.profile-login-item
  display: flex
  align-items: center
  gap: 6px
  &__icon
    color: var(--color-accent)
  &__label
    font-size: 10px
    color: var(--color-text-tertiary)
    white-space: nowrap
  &__value
    font-size: var(--text-xs)
    font-weight: 600
    color: var(--color-text-primary)
    font-variant-numeric: tabular-nums
    white-space: nowrap

// ── 内容卡 ──
.profile-card
  border-radius: 14px

.profile-card--fill
  height: 100%

.profile-card__header
  display: flex
  align-items: center
  gap: 8px
  padding: 10px 14px

.profile-card__icon
  display: inline-flex
  align-items: center
  justify-content: center
  width: 26px
  height: 26px
  border-radius: 7px
  color: var(--color-accent)
  background: color-mix(in srgb, var(--color-accent) 12%, transparent)
  border: 1px solid color-mix(in srgb, var(--color-accent) 22%, transparent)

.profile-card__title
  font-size: var(--text-sm)
  font-weight: 700
  color: var(--color-text-heading)

.profile-form
  padding: 14px
  :deep(.q-field__prepend .q-icon)
    color: var(--color-text-tertiary)
  :deep(.q-field--focus .q-field__prepend .q-icon)
    color: var(--color-accent)

// ── 安全设置紧凑列表 ──
.sec-item
  display: flex
  align-items: flex-start
  gap: 10px
  padding: 12px 14px

.sec-item__icon
  display: inline-flex
  align-items: center
  justify-content: center
  width: 26px
  height: 26px
  margin-top: 1px
  border-radius: 7px
  flex-shrink: 0
  color: var(--color-accent)
  background: color-mix(in srgb, var(--color-accent) 10%, transparent)
  border: 1px solid color-mix(in srgb, var(--color-accent) 18%, transparent)

.sec-item__body
  flex: 1 1 auto
  min-width: 0

.sec-item__title
  display: flex
  align-items: center
  gap: 8px
  font-size: var(--text-sm)
  font-weight: 600
  color: var(--color-text-heading)

.sec-item__badge
  font-size: 10px

.sec-item__hint
  margin-top: 2px
  font-size: var(--text-xs)
  line-height: 1.5
  color: var(--color-text-secondary)

.sec-item__form
  margin-top: 10px

.sec-item__form-actions
  display: flex
  justify-content: flex-end
  gap: 8px
  margin-top: 8px

.sec-item__action
  flex-shrink: 0
  margin-top: 1px

// ── 夜间科幻增强 ──
body.body--dark
  .profile-hero__scanline
    opacity: 0.9
    box-shadow: 0 0 12px color-mix(in srgb, var(--color-accent) 60%, transparent)
  .profile-hero__avatar
    box-shadow: 0 0 14px color-mix(in srgb, var(--color-accent) 35%, transparent)

@media (max-width: 1023px)
  .profile-hero__side
    width: 100%
    margin-left: 0
    justify-content: space-between
  .profile-hero__login
    padding-left: 0
    border-left: none

@media (max-width: 599px)
  .profile-hero__side
    flex-wrap: wrap
</style>
