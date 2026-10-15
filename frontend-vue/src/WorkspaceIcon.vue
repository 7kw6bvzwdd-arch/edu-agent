<script setup>
import {IconHome2,IconPresentation,IconMovie,IconBooks,IconFolders,IconWallet,IconChartBar,IconSettings,IconSparkles,IconArrowRight,IconHistory,IconCheck,IconUser,IconTrash,IconMenu2,IconTools,IconLogout,IconLock,IconArrowLeft,IconDownload,IconPencil,IconMessageCircle,IconArrowUp,IconArrowDown,IconPlus,IconCopy} from '@tabler/icons-vue'
defineProps({name:{type:String,required:true}})
const icons={home:IconHome2,slides:IconPresentation,animation:IconMovie,library:IconBooks,works:IconFolders,billing:IconWallet,dashboard:IconChartBar,settings:IconSettings,sparkle:IconSparkles,arrow:IconArrowRight,clock:IconHistory,check:IconCheck,account:IconUser,trash:IconTrash,menu:IconMenu2,tools:IconTools,logout:IconLogout,lock:IconLock,left:IconArrowLeft,download:IconDownload,edit:IconPencil,chat:IconMessageCircle,up:IconArrowUp,down:IconArrowDown,plus:IconPlus,copy:IconCopy}
</script>
<template><component :is="icons[name]||IconSparkles" :size="24" :stroke="1.75" aria-hidden="true" focusable="false"/></template>
