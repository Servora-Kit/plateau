import type { RouteRecordRaw } from 'vue-router';

import { $t } from '#/locales';

const routes: RouteRecordRaw[] = [
  {
    component: () => import('#/views/oauth-clients/index.vue'),
    meta: {
      authority: ['manage_clients'],
      fullPathKey: false,
      icon: 'lucide:key-round',
      order: 3,
      title: $t('page.oauthClients.title'),
    },
    name: 'OAuthClients',
    path: '/oauth-clients',
  },
];

export default routes;
