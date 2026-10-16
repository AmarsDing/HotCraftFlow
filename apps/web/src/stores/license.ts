import { defineStore } from "pinia";
import { ref } from "vue";

/** FrameWork stub: license gating disabled */
export const useLicenseStore = defineStore("license", () => {
  const restricted = ref(false);
  function setRestricted(v: boolean) {
    restricted.value = !!v;
  }
  return { restricted, setRestricted };
});
