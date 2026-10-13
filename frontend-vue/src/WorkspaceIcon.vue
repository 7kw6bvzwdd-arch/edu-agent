<script setup>
import {IconHome2,IconPresentation,IconMovie,IconBooks,IconFolders,IconWallet,IconChartBar,IconSettings,IconSparkles,IconArrowRight,IconHistory,IconCheck,IconUser,IconTrash} from '@tabler/icons-vue'
defineProps({name:{type:String,required:true}})
const icons={home:IconHome2,slides:IconPresentation,animation:IconMovie,library:IconBooks,works:IconFolders,billing:IconWallet,dashboard:IconChartBar,settings:IconSettings,sparkle:IconSparkles,arrow:IconArrowRight,clock:IconHistory,check:IconCheck,account:IconUser,trash:IconTrash}
</script>
<template><component :is="icons[name]||IconSparkles" :size="24" :stroke="1.75" aria-hidden="true" focusable="false"/></template>
