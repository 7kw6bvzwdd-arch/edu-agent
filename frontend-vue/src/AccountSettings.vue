<script setup>
import {ref,reactive,onMounted} from 'vue'
const props=defineProps({api:{type:Function,required:true}})
const emit=defineEmits(['renamed','password-changed'])
const profile=reactive({username:'',name:''}),password=reactive({current:'',next:'',confirm:''})
const busy=ref(false),ready=ref(false),error=ref(''),notice=ref('')
async function action(fn){if(busy.value)return;busy.value=true;error.value='';notice.value='';try{await fn()}catch(e){error.value=e.message}finally{busy.value=false}}
onMounted(()=>action(async()=>{Object.assign(profile,await props.api('/account'));ready.value=true}))
async function saveName(){await action(async()=>{const v=await props.api('/account','PATCH',{name:profile.name.trim()});profile.name=v.name;emit('renamed',v.name);notice.value='账户名称已保存。'})}
async function savePassword(){await action(async()=>{
 if(password.next!==password.confirm)throw Error('两次输入的新密码不一致。')
 const length=new TextEncoder().encode(password.next).length
 if(length<10||length>72)throw Error('新密码需为10–72字节，中文字符通常占3字节。')
 await props.api('/account/password','POST',{current_password:password.current,new_password:password.next})
 password.current=password.next=password.confirm='';emit('password-changed')
})}
</script>
<template>
 <section class="account-settings">
  <div class="page-heading"><div><div class="eyebrow">YOUR ACCOUNT</div><h1>个人账户</h1><p>管理账户名称与登录密码；在所有企业空间中同步生效。</p></div></div>
  <p v-if="error" class="alert error" role="alert">{{error}}</p><p v-if="notice" class="alert success" role="status">{{notice}}</p>
  <div class="account-grid">
   <form class="panel padded" @submit.prevent="saveName" aria-label="修改账户名称">
    <h2>账户信息</h2><label>登录账号<input :value="profile.username" readonly autocomplete="username"></label>
    <label>账户名称<input v-model="profile.name" required maxlength="30" autocomplete="name" placeholder="填写你希望显示的名称"></label>
    <p class="fine">名称用于头像旁、成员列表等位置；登录账号保持不变。</p>
    <button class="primary" :disabled="busy||!ready||!profile.name.trim()">保存名称</button>
   </form>
   <form class="panel padded" @submit.prevent="savePassword" aria-label="修改登录密码">
    <h2>修改密码</h2>
    <label>当前密码<input v-model="password.current" type="password" required autocomplete="current-password"></label>
    <label>新密码<input v-model="password.next" type="password" required maxlength="72" autocomplete="new-password"></label>
    <label>确认新密码<input v-model="password.confirm" type="password" required maxlength="72" autocomplete="new-password"></label>
    <p class="fine">10–72字节。修改后所有设备的旧会话退出，请用新密码重新登录。作品与额度不受影响。</p>
    <button class="primary" :disabled="busy||!ready">更新密码并重新登录</button>
   </form>
  </div>
 </section>
</template>
<style scoped>
.account-grid{display:grid;grid-template-columns:1fr 1fr;gap:24px;max-width:1000px}.account-grid label{display:block;margin:20px 0}.account-grid h2{margin-top:0}.account-grid button{margin-top:12px}.account-grid input{width:100%}@media(max-width:700px){.account-grid{grid-template-columns:1fr;gap:16px}}
</style>
