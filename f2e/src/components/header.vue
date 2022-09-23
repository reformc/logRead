<template>
	<div :class="cls">
		<slot></slot>
		<div v-if="status === 0" class="header-alert">连接中...</div>
		<div v-else-if="status === 1" class="header-alert">连接成功</div>
		<div v-else-if="status === 2" class="header-alert">连接断开中... </div>
		<div v-else-if="status === 3" @click="onOpen" class="header-alert">连接断开... <a>点击重连</a></div>
	</div>
</template>

<script setup>
	import ws from '../utils/ws.js'
	import {ref, computed} from 'vue';
	import { Notification } from '@arco-design/web-vue'

	const mapCls={
		0: 'info',
		1: '',
		2: 'warn',
		3: 'error'
	};
	const status = ref(ws.status);
	ws.on('close',()=>{
		status.value = ws.status;
	})
	ws.on('open', ()=>{
		status.value = ws.status;
	})
	ws.on('error', (e)=>{
		Notification.error(e||'ws连接出错了');
	})
	const cls = computed(()=>{
		return ['header',mapCls[status.value]].join(' ') ;
	})

	const onOpen = ()=>{
		return ws.open()
	}
</script>

<style lang="less">
.header{
	display: flex;
	top: 61px;
	z-index: 980;
	height: 32px;
	color: var(--color-white);
	line-height: 32px;
	background-color: rgb(var(--green-6));
	justify-content: space-between;
	&-alert{
		flex:1;
		text-align: center;
	}
	&.info{
		background: rgba(var(--blue-6));
	}
	&.error{
		background: rgba(var(--red-6));
	}
	&.warn{
		background: rgba(var(--orange-6));
	}
	.arco-btn-text{
		color: var(--color-white);
	}
}
</style>