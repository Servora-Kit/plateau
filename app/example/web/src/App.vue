<script setup lang="ts">
import { RouterView } from 'vue-router'
</script>

<template>
  <div class="app-shell">
    <header class="site-header">
      <a class="brand" href="/" aria-label="Servora CRUD Console 首页">
        <span class="brand-mark" aria-hidden="true">S/</span>
        <span>Servora CRUD Console</span>
      </a>
      <div class="runtime-badge">
        <span class="runtime-badge__pulse" aria-hidden="true"></span>
        example.service · localhost:28080
      </div>
    </header>
    <main>
      <RouterView />
    </main>
    <footer class="site-footer">
      <span>Generated contracts, exercised live.</span>
      <code>example.servora.dev/User</code>
    </footer>
  </div>
</template>
