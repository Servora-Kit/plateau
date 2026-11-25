import type { RouteRecordRaw } from 'vue-router';

import { $t } from '#/locales';

const routes: RouteRecordRaw[] = [
  {
    component: () => import('#/views/audit/index.vue'),
    meta: {
      authority: ['read_audit'],
      fullPathKey: false,
      icon: 'lucide:scroll-text',
      order: 3,
      title: $t('page.audit.title'),
    },
    name: 'AuditLogs',
    path: '/audit',
  },
];

export default routes;
