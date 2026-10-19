<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { useAuthStore } from '@/stores/auth'
import PgIcon from '@/components/ui/PgIcon.vue'
import PgBadge from '@/components/ui/PgBadge.vue'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive({
  username: '',
  password: '',
})

const rules: FormRules = {
  username: [{ required: true, message: '请输入平台用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入平台访问密码', trigger: 'blur' }],
}

function fillDemoAccount() {
  form.username = 'platform'
  form.password = 'piguard-platform-http-demo'
  ElMessage.info('已填入课设默认测试账号')
}

async function submit() {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    loading.value = true
    try {
      await auth.login(form.username, form.password)
      ElMessage.success('登录成功，正在进入座舱控制台')
      const redirect = (route.query.redirect as string) || '/dashboard'
      router.replace(redirect)
    } catch {
      /* 错误已由 request 拦截器统一提示 */
    } finally {
      loading.value = false
    }
  })
}
</script>

<template>
  <div class="pg-login-page">
    <!-- 背景流光与网格氛围 -->
    <div class="pg-bg-grid" />
    <div class="pg-bg-radial pg-bg-radial--top" />
    <div class="pg-bg-radial pg-bg-radial--bottom" />

    <div class="pg-login-container pg-stagger">
      <!-- 平台卡片 -->
      <div class="pg-login-card">
        <!-- 头部品牌 -->
        <div class="pg-login-header">
          <div class="pg-brand-badge">
            <div class="pg-brand-glow" />
            <div class="pg-brand-icon">
              <PgIcon name="shield" :size="30" :stroke="2.2" />
            </div>
          </div>
          <h1 class="pg-login-title">PiGuard ADAS</h1>
          <p class="pg-login-desc">基于 Raspberry Pi 4B 的车载驾驶辅助预警平台</p>
          <div class="pg-badge-row">
            <PgBadge tone="primary" size="sm" :dot="true">Go 平台引擎</PgBadge>
            <PgBadge tone="ok" size="sm" :dot="true">边缘端传感器链</PgBadge>
            <PgBadge tone="info" size="sm">MQTT / HTTP / WS</PgBadge>
          </div>
        </div>

        <!-- 登录表单 -->
        <el-form
          ref="formRef"
          :model="form"
          :rules="rules"
          size="large"
          class="pg-login-form"
          @keyup.enter="submit"
        >
          <el-form-item prop="username">
            <el-input
              v-model="form.username"
              placeholder="平台用户名 (如: platform)"
              autocomplete="username"
            >
              <template #prefix>
                <PgIcon name="user" :size="16" class="pg-input-icon" />
              </template>
            </el-input>
          </el-form-item>

          <el-form-item prop="password">
            <el-input
              v-model="form.password"
              type="password"
              placeholder="访问凭证密码"
              show-password
              autocomplete="current-password"
            >
              <template #prefix>
                <PgIcon name="lock" :size="16" class="pg-input-icon" />
              </template>
            </el-input>
          </el-form-item>

          <el-button
            type="primary"
            class="pg-submit-btn"
            :loading="loading"
            @click="submit"
          >
            <template v-if="!loading">
              <span>进入座舱系统</span>
              <PgIcon name="chevronRight" :size="16" />
            </template>
            <template v-else>正在验证安全凭据...</template>
          </el-button>
        </el-form>

        <!-- 快捷操作与环境提示 -->
        <div class="pg-login-footer">
          <button class="pg-quick-fill-btn" type="button" @click="fillDemoAccount">
            <PgIcon name="zap" :size="13" />
            <span>快捷填入课设演示账号</span>
          </button>
          <p class="pg-security-tip">
            请使用平台管理账号登录；边缘设备端账号受权限隔离，无法访问管理控制台。
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pg-login-page {
  position: relative;
  width: 100vw;
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--pg-bg-deep);
  overflow: hidden;
  padding: 20px;
}

/* 背景 HUD 网格效果 */
.pg-bg-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(148, 163, 184, 0.04) 1px, transparent 1px),
    linear-gradient(90deg, rgba(148, 163, 184, 0.04) 1px, transparent 1px);
  background-size: 40px 40px;
  mask-image: radial-gradient(circle at center, black 40%, transparent 85%);
  -webkit-mask-image: radial-gradient(circle at center, black 40%, transparent 85%);
  pointer-events: none;
}

.pg-bg-radial {
  position: absolute;
  width: 60vw;
  height: 60vw;
  border-radius: 50%;
  pointer-events: none;
  filter: blur(100px);
}

.pg-bg-radial--top {
  top: -20vw;
  left: -10vw;
  background: radial-gradient(circle, rgba(15, 181, 214, 0.12) 0%, transparent 70%);
}

.pg-bg-radial--bottom {
  bottom: -20vw;
  right: -10vw;
  background: radial-gradient(circle, rgba(99, 102, 241, 0.1) 0%, transparent 70%);
}

.pg-login-container {
  width: 100%;
  max-width: 440px;
  position: relative;
  z-index: 1;
}

.pg-login-card {
  background: rgba(15, 23, 38, 0.78);
  border: 1px solid var(--pg-border-strong);
  border-radius: var(--pg-radius-lg);
  padding: 38px 32px 30px;
  backdrop-filter: blur(24px) saturate(160%);
  -webkit-backdrop-filter: blur(24px) saturate(160%);
  box-shadow: 0 30px 80px -15px rgba(0, 0, 0, 0.8), var(--pg-inset-highlight);
  position: relative;
}

.pg-login-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 30px;
  right: 30px;
  height: 2px;
  background: var(--pg-gradient-brand);
  border-radius: 999px;
  opacity: 0.8;
}

.pg-login-header {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  margin-bottom: 28px;
}

.pg-brand-badge {
  position: relative;
  width: 64px;
  height: 64px;
  margin-bottom: 16px;
}

.pg-brand-glow {
  position: absolute;
  inset: -6px;
  border-radius: 20px;
  background: var(--pg-gradient-brand);
  opacity: 0.45;
  filter: blur(12px);
  animation: pg-pulse 3s infinite ease-in-out;
}

.pg-brand-icon {
  position: relative;
  width: 100%;
  height: 100%;
  border-radius: 18px;
  background: linear-gradient(135deg, #0fb5d6, #3b82f6);
  display: grid;
  place-items: center;
  color: #fff;
  border: 1px solid rgba(255, 255, 255, 0.2);
  box-shadow: 0 8px 24px -4px rgba(15, 181, 214, 0.5);
}

.pg-login-title {
  font-size: 24px;
  font-weight: 750;
  letter-spacing: -0.02em;
  color: #fff;
  margin-bottom: 6px;
}

.pg-login-desc {
  font-size: 13px;
  color: var(--pg-text-muted);
  max-width: 320px;
  line-height: 1.4;
  margin-bottom: 14px;
}

.pg-badge-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: center;
}

.pg-login-form {
  margin-bottom: 20px;
}

.pg-input-icon {
  color: var(--pg-text-faint);
  margin-right: 4px;
}

.pg-submit-btn {
  width: 100%;
  height: 44px;
  font-size: 14.5px;
  font-weight: 650;
  border-radius: var(--pg-radius-sm);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  margin-top: 6px;
}

.pg-login-footer {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  border-top: 1px solid var(--pg-border);
  padding-top: 16px;
}

.pg-quick-fill-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: rgba(34, 211, 238, 0.08);
  border: 1px dashed rgba(34, 211, 238, 0.35);
  color: var(--pg-primary);
  font-size: 12px;
  padding: 6px 12px;
  border-radius: var(--pg-radius-pill);
  cursor: pointer;
  transition: all var(--pg-dur-fast) var(--pg-ease);
}

.pg-quick-fill-btn:hover {
  background: rgba(34, 211, 238, 0.16);
  border-color: var(--pg-primary);
  transform: translateY(-1px);
}

.pg-security-tip {
  font-size: 11.5px;
  color: var(--pg-text-faint);
  text-align: center;
  line-height: 1.4;
  max-width: 340px;
}
</style>
