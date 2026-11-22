<script lang="ts" setup>
import { computed } from 'vue';

import { AuthPageLayout } from '@vben/layouts';
import { preferences } from '@vben/preferences';

const appName = computed(() => preferences.app.name);
const logo = computed(() => preferences.logo.source);
const logoDark = computed(() => preferences.logo.sourceDark);
</script>

<template>
  <AuthPageLayout
    :app-name="appName"
    :copyright="false"
    :logo="logo"
    :logo-dark="logoDark"
    page-description="基于 Plateau IAM 统一身份认证与 OpenFGA 实时授权的平台管理控制台"
    page-title="Plateau 平台管理工作台"
    :toolbar-list="['theme']"
  />
</template>
