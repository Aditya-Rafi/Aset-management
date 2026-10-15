import { createRouter, createWebHistory } from 'vue-router'
import { useAuth } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue'), meta: { guest: true, title: 'Login' } },
    {
      path: '/',
      component: () => import('@/layouts/AppLayout.vue'),
      meta: { auth: true },
      children: [
        { path: '', redirect: { name: 'dashboard' } },
        { path: 'dashboard', name: 'dashboard', component: () => import('@/views/DashboardView.vue'), meta: { title: 'Dashboard' } },
        { path: 'asset', name: 'asset', component: () => import('@/views/AssetsView.vue'), meta: { title: 'Daftar Aset' } },
        // the address inside every QR sticker
        { path: 'a/:tag', name: 'asset.detail', component: () => import('@/views/AssetDetailView.vue'), meta: { title: 'Detail Aset' } },
        { path: 'scan', name: 'scan', component: () => import('@/views/ScanView.vue'), meta: { title: 'Scan Kode' } },
        { path: 'labels', name: 'labels', component: () => import('@/views/LabelsView.vue'), meta: { title: 'Cetak Label' } },
        { path: 'handover/new', name: 'handover.create', component: () => import('@/views/HandoverCreateView.vue'), meta: { title: 'Buat Dokumen Serah Terima' } },
        { path: 'handover', name: 'handover', component: () => import('@/views/HandoverListView.vue'), meta: { title: 'Riwayat Serah Terima' } },
        { path: 'employee', name: 'employee', component: () => import('@/views/EmployeesView.vue'), meta: { title: 'Karyawan' } },
        { path: 'category', name: 'category', component: () => import('@/views/CategoriesView.vue'), meta: { title: 'Kategori' } },
        { path: 'monitoring/employee', name: 'monitor.employee', component: () => import('@/views/MonitorEmployeeView.vue'), meta: { title: 'Inventaris per Karyawan' } },
        { path: 'activity', name: 'activity', component: () => import('@/views/ActivityView.vue'), meta: { title: 'Log Aktivitas' } },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuth()
  if (!auth.ready) await auth.fetchMe()
  // Not signed in (e.g. a phone just scanned a sticker): sign in, then land on that asset.
  if (to.meta.auth && !auth.user) return { name: 'login', query: to.fullPath === '/' ? {} : { redirect: to.fullPath } }
  if (to.meta.guest && auth.user) return { name: 'dashboard' }
})

router.afterEach((to) => {
  document.title = (to.meta.title ? to.meta.title + ' · ' : '') + 'Aset IT'
})

export default router
