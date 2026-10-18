<template>
  <q-page class="q-pa-md">
    <div class="row items-center q-mb-md">
      <div class="text-h5">MME 混剪任务</div>
      <q-space />
      <q-btn flat dense round icon="refresh" :loading="loading" @click="fetchData" />
      <q-btn unelevated color="primary" no-caps icon="add" label="新建任务" @click="createOpen = true" />
    </div>

    <!-- 基础设施体检卡（缺依赖诚实红标；required 缺失时 ready=false） -->
    <q-card flat bordered class="q-mb-md" :class="doctor && !doctor.ready ? 'bg-red-1' : 'bg-green-1'">
      <q-card-section class="q-py-sm">
        <div class="row items-center q-gutter-sm">
          <q-icon
            :name="doctor && doctor.ready ? 'verified' : 'report_problem'"
            :color="doctor && doctor.ready ? 'positive' : 'negative'"
          />
          <div class="text-subtitle2">基础设施体检</div>
          <q-badge v-if="doctor" :color="doctor.ready ? 'positive' : 'negative'" outline>
            {{ doctor.ready ? '依赖齐备' : '存在缺失依赖' }}
          </q-badge>
          <q-space />
          <q-btn
            flat
            dense
            no-caps
            size="sm"
            icon="medical_services"
            label="重新体检"
            :loading="doctorLoading"
            @click="fetchDoctor"
          />
        </div>
        <div v-if="doctor" class="row q-gutter-xs q-mt-xs">
          <q-chip
            v-for="it in doctor.items || []"
            :key="it.name"
            dense
            :color="it.ok ? 'green-2' : it.required ? 'red-2' : 'orange-2'"
            :text-color="it.ok ? 'green-10' : it.required ? 'red-10' : 'orange-10'"
            :icon="it.ok ? 'check' : it.required ? 'close' : 'warning'"
          >
            {{ it.name }}<template v-if="it.version">（{{ it.version }}）</template>
            <q-tooltip v-if="!it.ok"
              >{{ it.detail || '不可用' }}{{ it.required ? '（必需）' : '（可选，相应功能降级）' }}</q-tooltip
            >
          </q-chip>
        </div>
        <div v-if="doctor && !doctor.ready" class="text-caption text-negative q-mt-xs">
          必需依赖缺失将导致下载/渲染如实失败（MME_DOWNLOAD_FAIL / MME_RENDER_FAIL）；可选依赖缺失时
          ASR/文案功能降级为手工 EDL 与模板文案，不伪造产出。
        </div>
      </q-card-section>
    </q-card>

    <div class="row q-col-gutter-sm q-mb-sm">
      <div class="col-auto">
        <q-select
          v-model="stateFilter"
          dense
          outlined
          :options="stateOptions"
          emit-value
          map-options
          label="状态筛选"
          style="min-width: 160px"
          @update:model-value="onSearch"
        />
      </div>
      <div class="col" style="max-width: 320px">
        <q-input
          v-model="keyword"
          dense
          outlined
          clearable
          placeholder="按标题搜索"
          @keyup.enter="onSearch"
          @clear="onSearch"
        >
          <template #append><q-icon name="search" class="cursor-pointer" @click="onSearch" /></template>
        </q-input>
      </div>
    </div>

    <q-table flat bordered :rows="items" :columns="columns" row-key="id" :loading="loading" hide-bottom>
      <template #body-cell-title="props">
        <q-td :props="props">
          <a class="text-primary cursor-pointer" @click="goDetail(props.row)">{{
            props.row.title || shortId(props.row.id)
          }}</a>
        </q-td>
      </template>
      <template #body-cell-sourceType="props">
        <q-td :props="props">
          <q-badge outline :color="props.row.sourceType === 'url' ? 'primary' : 'secondary'">{{
            props.row.sourceType
          }}</q-badge>
        </q-td>
      </template>
      <template #body-cell-state="props">
        <q-td :props="props">
          <q-badge :color="taskStateMeta(props.row.state).color" outline>
            <q-icon :name="taskStateMeta(props.row.state).icon" size="14px" class="q-mr-xs" />
            {{ taskStateMeta(props.row.state).label }}
          </q-badge>
          <q-tooltip v-if="props.row.errorMsg">{{ props.row.errorMsg }}</q-tooltip>
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-btn flat dense round icon="visibility" @click="goDetail(props.row)"
            ><q-tooltip>详情 / 手工 EDL</q-tooltip></q-btn
          >
          <q-btn flat dense round icon="psychology" :loading="actingId === props.row.id" @click="doAnalyze(props.row)">
            <q-tooltip>触发分析（ASR→高光；缺依赖如实报错）</q-tooltip>
          </q-btn>
          <q-btn flat dense round color="negative" icon="delete" @click="confirmDelete(props.row)"
            ><q-tooltip>删除任务（保留底片）</q-tooltip></q-btn
          >
        </q-td>
      </template>
      <template #no-data>
        <div class="row justify-center q-pa-lg text-grey-7">
          <div class="text-center">
            <q-icon name="smart_display" size="48px" class="q-mb-sm" />
            <div>暂无混剪任务，点击右上角「新建任务」开始</div>
          </div>
        </div>
      </template>
    </q-table>

    <div class="row justify-center q-mt-md">
      <q-pagination v-model="page" :max="maxPage" direction-links boundary-numbers @update:model-value="fetchData" />
    </div>

    <!-- 新建任务对话框 -->
    <q-dialog v-model="createOpen" persistent>
      <q-card style="min-width: 460px">
        <q-card-section class="text-h6">新建混剪任务</q-card-section>
        <q-card-section class="q-gutter-md">
          <q-input
            v-model="form.title"
            outlined
            dense
            label="任务标题"
            autofocus
            :rules="[(v: string) => !!v?.trim() || '标题必填']"
          />
          <q-select
            v-model="form.sourceType"
            outlined
            dense
            :options="[
              { label: 'URL（yt-dlp 下载）', value: 'url' },
              { label: '本地文件路径（服务端入库）', value: 'local' },
            ]"
            emit-value
            map-options
            label="来源类型"
          />
          <q-input
            v-model="form.sourceRef"
            outlined
            dense
            :label="form.sourceType === 'url' ? '视频 URL' : '服务端本地绝对路径'"
            :hint="
              form.sourceType === 'url'
                ? '由 yt-dlp 下载入素材库（指纹去重）'
                : '该路径须在运行 admin 服务的机器上可读；文件复制入素材库'
            "
            :rules="[(v: string) => !!v?.trim() || '来源必填']"
          />
          <q-input
            v-model="form.userIntent"
            outlined
            dense
            type="textarea"
            autogrow
            label="剪辑意图（可选，供 LLM 分析参考）"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps label="取消" @click="createOpen = false" />
          <q-btn
            unelevated
            no-caps
            color="primary"
            label="创建"
            :disable="!form.title.trim() || !form.sourceRef.trim()"
            :loading="creating"
            @click="doCreate"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Dialog, Notify } from 'quasar';
import { mmeTaskService } from '../../../services/index';
import { errMessage, fmtTime, shortId, taskStateMeta, type DoctorReport, type MontageTask } from './types';

const router = useRouter();

const items = ref<MontageTask[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = 20;
const loading = ref(false);
const actingId = ref('');
const stateFilter = ref('');
const keyword = ref('');

const doctor = ref<DoctorReport | null>(null);
const doctorLoading = ref(false);

const maxPage = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

const stateOptions = [
  { label: '全部状态', value: '' },
  { label: '已创建', value: 'created' },
  { label: '下载中', value: 'downloading' },
  { label: '转写中', value: 'transcribing' },
  { label: '分析中', value: 'analyzing' },
  { label: '已展开', value: 'expanded' },
  { label: '渲染中', value: 'rendering' },
  { label: '待审核', value: 'reviewing' },
  { label: '完成', value: 'done' },
  { label: '失败', value: 'failed' },
];

const columns = [
  { name: 'title', label: '标题', field: 'title', align: 'left' as const },
  { name: 'sourceType', label: '来源', field: 'sourceType', align: 'left' as const },
  {
    name: 'sourceRef',
    label: '来源引用',
    field: (r: MontageTask) => r.sourceRef || '-',
    align: 'left' as const,
    style: 'max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;',
  },
  { name: 'state', label: '状态', field: 'state', align: 'left' as const },
  { name: 'createdAt', label: '创建时间', field: (r: MontageTask) => fmtTime(r.createdAt), align: 'left' as const },
  { name: 'actions', label: '操作', field: 'id', align: 'right' as const },
];

async function fetchDoctor() {
  doctorLoading.value = true;
  try {
    doctor.value = await mmeTaskService.doctor();
  } catch (e) {
    Notify.create({ type: 'warning', message: `体检失败：${errMessage(e)}` });
  } finally {
    doctorLoading.value = false;
  }
}

async function fetchData() {
  loading.value = true;
  try {
    const formValues: Record<string, unknown> = {};
    if (stateFilter.value) formValues.state = stateFilter.value;
    if (keyword.value.trim()) formValues.title = keyword.value.trim();
    const res = await mmeTaskService.list({ page: page.value, pageSize, formValues });
    items.value = res.items ?? [];
    total.value = res.total ?? 0;
  } catch (e) {
    Notify.create({ type: 'negative', message: errMessage(e) });
  } finally {
    loading.value = false;
  }
}

function onSearch() {
  page.value = 1;
  fetchData();
}

function goDetail(row: MontageTask) {
  router.push(`/media/mme/tasks/${row.id}`);
}

async function doAnalyze(row: MontageTask) {
  actingId.value = row.id ?? '';
  try {
    const t = await mmeTaskService.analyze(row.id ?? '');
    Notify.create({ type: 'positive', message: `分析已触发，当前状态：${taskStateMeta(t.state).label}` });
    await fetchData();
  } catch (e) {
    // MME_QUEUE_UNAVAILABLE / MME_ASR_FAIL 等如实上屏
    Notify.create({ type: 'negative', timeout: 6000, message: errMessage(e) });
  } finally {
    actingId.value = '';
  }
}

function confirmDelete(row: MontageTask) {
  Dialog.create({
    title: '删除任务确认',
    message: `确认删除「${row.title || row.id}」？素材底片按引用计数保留，不被删除。`,
    cancel: { flat: true, noCaps: true, label: '取消' },
    ok: { unelevated: true, noCaps: true, color: 'negative', label: '删除' },
  }).onOk(async () => {
    try {
      await mmeTaskService.remove(row.id ?? '');
      Notify.create({ type: 'positive', message: '已删除' });
      await fetchData();
    } catch (e) {
      Notify.create({ type: 'negative', message: errMessage(e) });
    }
  });
}

const createOpen = ref(false);
const creating = ref(false);
const form = ref({ title: '', sourceType: 'url', sourceRef: '', userIntent: '' });

async function doCreate() {
  creating.value = true;
  try {
    const t = await mmeTaskService.create({
      title: form.value.title.trim(),
      sourceType: form.value.sourceType,
      sourceRef: form.value.sourceRef.trim(),
      userIntent: form.value.userIntent.trim() || undefined,
    });
    Notify.create({ type: 'positive', message: '任务已创建' });
    createOpen.value = false;
    form.value = { title: '', sourceType: 'url', sourceRef: '', userIntent: '' };
    router.push(`/media/mme/tasks/${t.id}`);
  } catch (e) {
    Notify.create({ type: 'negative', timeout: 6000, message: errMessage(e) });
  } finally {
    creating.value = false;
  }
}

onMounted(() => {
  fetchDoctor();
  fetchData();
});
</script>
