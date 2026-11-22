import type { RouteRecordRaw } from 'vue-router';

import { $t } from '#/locales';

const routes: RouteRecordRaw[] = [
  {
    component: () => import('#/views/users/index.vue'),
    meta: {
      fullPathKey: false,
      icon: 'lucide:users',
      order: 2,
      title: $t('page.users.title'),
    },
    name: 'Users',
    path: '/users',
  },
];

export default routes;
