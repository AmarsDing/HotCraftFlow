import { defineBoot } from '#q-app/wrappers';
import QPopupConfirm from '../components/QPopupConfirm.vue';

export default defineBoot(({ app }) => {
  app.component('QPopupConfirm', QPopupConfirm);
});
