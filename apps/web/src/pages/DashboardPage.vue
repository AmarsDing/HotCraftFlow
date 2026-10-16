<template>
  <q-page class="q-pa-lg">
    <div class="text-h5 q-mb-md">{{ t("nav.dashboard") || "Dashboard" }}</div>
    <div class="text-body1 text-grey-7 q-mb-lg">
      Framework platform skeleton is running. Use the side menu to open organization, permission, system and log pages.
    </div>
    <div class="row q-col-gutter-md">
      <div v-for="card in cards" :key="card.to" class="col-12 col-sm-6 col-md-3">
        <q-card flat bordered class="cursor-pointer" @click="go(card.to)">
          <q-card-section class="row items-center q-gutter-sm">
            <q-icon :name="card.icon" size="28px" color="primary" />
            <div>
              <div class="text-subtitle1">{{ card.title }}</div>
              <div class="text-caption text-grey">{{ card.to }}</div>
            </div>
          </q-card-section>
        </q-card>
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";

defineOptions({ name: "DashboardPage" });

const { t } = useI18n();
const router = useRouter();

const cards = [
  { title: "Users", icon: "person", to: "/opm/users" },
  { title: "Roles", icon: "admin_panel_settings", to: "/permission/roles" },
  { title: "Menus", icon: "menu", to: "/permission/menus" },
  { title: "Dicts", icon: "book", to: "/system/dicts" },
  { title: "Configs", icon: "tune", to: "/system/configs" },
  { title: "Tenant", icon: "apartment", to: "/tenant" },
  { title: "Login Log", icon: "login", to: "/log/login" },
  { title: "Operation Log", icon: "history_edu", to: "/log/operation" },
];

function go(to: string) {
  void router.push(to);
}
</script>