import type { RouteRecordRaw } from 'vue-router';

import { $t } from '#/locales';

const routes: RouteRecordRaw[] = [
  {
    component: () => import('#/views/overview/index.vue'),
    meta: {
      affixTab: true,
      icon: 'lucide:layout-dashboard',
      order: 1,
      title: $t('page.overview.title'),
    },
    name: 'Overview',
    path: '/overview',
  },
];

export default routes;
