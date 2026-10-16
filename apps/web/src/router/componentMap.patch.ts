/**
 * PATCH for apps/web/src/router/componentMap.ts
 *
 * Add this entry into the componentMap object (do not replace the whole file):
 *
 *   'github-radar/RadarPage': () => import('../pages/github-radar/RadarPage.vue'),
 *
 * Menu seed component string must be exactly: github-radar/RadarPage
 */
export const githubRadarComponentMapEntry = {
  'github-radar/RadarPage': () => import('../pages/github-radar/RadarPage.vue'),
};
