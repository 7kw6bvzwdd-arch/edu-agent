<script setup>
import {ref,reactive} from 'vue'
const props=defineProps({api:Function,health:Object,notice:String})
const emit=defineEmits(['authenticated'])
const mode=ref('login'),busy=ref(false),error=ref(''),result=ref(null),saved=ref(false),showPassword=ref(false)
const form=reactive({username:'',password:'',confirm:'',name:'',workspace:'',recovery_code:''})
function switchMode(next){mode.value=next;error.value='';form.password='';form.confirm='';form.recovery_code='';showPassword.value=false}
function checkPassword(){const n=new TextEncoder().encode(form.password).length;if(n<10||n>72||!form.password.trim())throw Error('密码需为10–72字节，不能全为空白。');if(form.password!==form.confirm)throw Error('两次输入的密码不一致。')}
async function submit(){if(busy.value)return;busy.value=true;error.value='';try{
 if(mode.value==='login'){const v=await props.api('/auth/login','POST',{username:form.username.trim(),password:form.password});form.password='';emit('authenticated',v)}
 else {checkPassword();const path=mode.value==='register'?'/auth/register':'/auth/reset-password';result.value=await props.api(path,'POST',{username:form.username.trim(),password:form.password,name:form.name.trim(),workspace:form.workspace.trim(),recovery_code:form.recovery_code.trim()});form.password=form.confirm=form.recovery_code='';saved.value=false}
 }catch(e){error.value=e.message}finally{busy.value=false}}
function downloadCode(){const file=new Blob(['备课工坊账户恢复码\n账号：'+form.username+'\n恢复码：'+result.value.recovery_code+'\n请保存在安全位置。该码可重置密码，每次使用后失效。'],{type:'text/plain;charset=utf-8'});const url=URL.createObjectURL(file),a=document.createElement('a');a.href=url;a.download='备课工坊-恢复码.txt';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}
</script>
<template>
 <section class="login-form auth-panel" aria-label="账户登录与恢复">
  <template v-if="result">
   <span class="eyebrow">KEEP YOUR ACCOUNT SAFE</span><h2>{{mode==='register'?'欢迎加入备课工坊':'密码重置成功'}}</h2><p>{{result.message}}</p>
   <div class="recovery-card"><b>请立即保存恢复码</b><p class="fine">仅在此处展示明文。忘记密码时凭此码验证身份；请勿分享。</p><code>{{result.recovery_code}}</code><button type="button" @click="downloadCode">下载恢复码</button></div>
   <label class="inline-check"><input type="checkbox" v-model="saved">我已安全保存恢复码</label><button class="primary wide" :disabled="!saved" @click="result=null;switchMode('login')">返回登录</button>
  </template>
  <form v-else @submit.prevent="submit" :aria-label="mode==='login'?'登录':mode==='register'?'注册账号':'找回密码'">
   <span class="eyebrow">{{mode==='login'?'WELCOME BACK':mode==='register'?'START YOUR TEACHING SPACE':'RECOVER YOUR ACCOUNT'}}</span>
   <h2>{{mode==='login'?'进入你的备课空间':mode==='register'?'创建你的备课账号':'找回登录密码'}}</h2>
   <p class="muted">{{mode==='login'?'每位老师独立创作，每份知识共同积累。':mode==='register'?'创建独立企业空间，已有企业的成员请联系管理员开通。':'使用注册时保存的恢复码验证身份，不会直接修改他人密码。'}}</p>
   <div v-if="mode!=='reset'" class="auth-tabs"><button type="button" :class="{active:mode==='login'}" @click="switchMode('login')" :disabled="busy">登录</button><button type="button" :class="{active:mode==='register'}" @click="switchMode('register')" :disabled="busy">注册账号</button></div>
   <label>账号<input v-model="form.username" required maxlength="40" autocomplete="username" placeholder="3–40位字母、数字或 . _ -" :pattern="mode==='register'?'[a-zA-Z0-9_.-]{3,40}':undefined"></label>
   <template v-if="mode==='register'"><label>显示名称<input v-model="form.name" required maxlength="30" autocomplete="name" placeholder="例如：林老师"></label><label>企业空间名称<input v-model="form.workspace" required maxlength="60" autocomplete="organization" placeholder="例如：我的教研工作室"></label></template>
   <label v-if="mode==='reset'">一次性恢复码<input v-model="form.recovery_code" required maxlength="128" autocomplete="off" placeholder="输入安全保存的恢复码"></label>
   <label>{{mode==='login'?'密码':'新密码'}}<div class="password-input"><input v-model="form.password" :type="showPassword?'text':'password'" :autocomplete="mode==='login'?'current-password':'new-password'" required :maxlength="mode==='login'?undefined:72" :placeholder="mode==='login'?'请输入登录密码':'10–72字节'"/><button type="button" :aria-label="showPassword?'隐藏密码':'显示密码'" :aria-pressed="showPassword" @click="showPassword=!showPassword">{{showPassword?'隐藏':'显示'}}</button></div></label>
   <label v-if="mode!=='login'">确认新密码<input v-model="form.confirm" type="password" required maxlength="72" autocomplete="new-password" placeholder="再次输入密码"></label>
   <div v-if="mode==='login'" class="auth-links"><button type="button" @click="switchMode('reset')" :disabled="busy">忘记密码？</button></div>
   <p v-if="error" class="alert error" role="alert">{{error}}</p><p v-if="notice&&mode==='login'" class="alert success" role="status">{{notice}}</p>
   <button class="primary wide" :disabled="busy">{{busy?'处理中…':mode==='login'?'登录工作台 →':mode==='register'?'注册并创建空间':'验证身份并重置密码'}}</button>
   <button v-if="mode==='reset'" type="button" class="wide" @click="switchMode('login')" :disabled="busy">返回登录</button>
   <p class="fine">{{mode==='register'?'注册不自动赠送积分。登录后可开通基础套餐（模拟支付）。':mode==='reset'?'重置后所有设备退出登录，旧恢复码失效。没有恢复码的旧账户请联系管理员；登录后可在个人账户领取。':health.mode==='sample'?'本地示例模式 · 不产生真实模型费用':'私有企业空间 · 账户与作品隔离'}}</p>
  </form>
 </section>
</template>
<style scoped>
.auth-panel{max-height:none}.auth-panel h2{margin:12px 0}.auth-panel label{margin:18px 0}.auth-tabs{display:flex;gap:6px;padding:5px;background:#eef2e9;border-radius:10px;margin:22px 0 10px}.auth-tabs button{flex:1;background:transparent;border:0}.auth-tabs button.active{background:white;box-shadow:0 2px 8px #273a3310}.auth-links{display:flex;justify-content:flex-end;margin:-5px 0 15px}.auth-links button{padding:2px 0;border:0;background:none;color:var(--green)}.password-input{display:flex;border:1px solid #dfe5dc;border-radius:7px;margin-top:8px;overflow:hidden}.password-input input{border:0;flex:1}.password-input button{border:0;font-size:12px;padding:8px}.auth-panel .wide+button{margin-top:12px}.recovery-card{padding:18px;border:1px solid #dce6d5;background:#f3f7ed;border-radius:12px;margin:24px 0}.recovery-card code{display:block;overflow-wrap:anywhere;user-select:all;background:white;padding:14px;margin:12px 0;font-size:13px;line-height:1.6}
</style>
