<script setup>
	import {ref, onMounted} from 'vue'
	import VirtualList from 'vue3-virtual-scroll-list'
	import ws from '../utils/ws.js'
	import {IconDown} from '@arco-design/web-vue/es/icon'
	import format from '../utils/format';
	import Item from './Item.vue';

	const target = ref(null);
	const visible = ref(false);
	const count  = ref(0);
	const lines = ref([]);
	const lineNumber = ref(10000);
	const codeVisible = ref(false);
	const codeTxt = ref('');
	let id = 0;

	const scrollBottom = (ev) => {
		if (!ev && visible.value) return;
		if (target.value) {
			const el = target.value;
			count.value = 0;
			el.scrollToBottom();
		}
	}
	onMounted(() => {
		ws.on('message', (data) => {
			const line = lines.value;
			data.map((txt) => {
				id++;
				line.push({ id, txt });
			});
			if(line.length> lineNumber) lines.value = line.slice(line.length-lineNumber);
			count.value += data.length;
			scrollBottom();
		})
		ws.on('channelChange',(data)=>{
			if(data.lines >0) lineNumber.value = data.lines;
			id=0;
			lines.value = [];
		})
	})

	const onScroll = (evt)=>{
		const el = evt.target;
		const {scrollTop, offsetHeight, scrollHeight} = el;
		if(scrollTop+ offsetHeight + 300 >= scrollHeight){
			visible.value = false;
		}else {
			visible.value = true;
		}
	}

	const onSelect = (e)=>{
		codeVisible.value = true;
		codeTxt.value = format(e.txt);
	}
	const onCodeHide = ()=>{
		codeVisible.value = false;
	}

</script>

<template>
	<div class="code">
		<div v-if="lines.length>0" class="code-title" >
			共: {{ lines.length }} 行
		</div>
		<VirtualList
				ref="target"
				class="code-box"
				:data-key="'id'"
				itemClass="code-line"
				:keeps="150"
				:extra-props="{
					onSelect: onSelect
				}"
				@scroll="onScroll"
				:data-sources="lines"
				:data-component="Item"
		>
		</VirtualList>
		<a-modal  width="85%" :footer="null" title="日志信息" v-model:visible="codeVisible" @ok="onCodeHide" @cancel="(e)=>onCodeHide">
			<div class="code-modal" v-html="codeTxt"></div>
		</a-modal>
		<a-badge v-if="visible" :count="count" class="back-top-btn">
			<a-button  @click="scrollBottom" >
				<IconDown/>
			</a-button>
		</a-badge>

	</div>
</template>

<style lang="less">
	.code{
		position: relative;
		height: calc(100vh - 32px);
		width: 100%;


		&-title{
			position: absolute;
			z-index: 100;
			padding: 6px 10px;
			background: rgb(var(--gray-7));
			color: var(--color-white);
			box-shadow: 0 3px 10px rgb(var(--gray-10));
		}
		&-line{
			box-sizing: border-box;
			background: rgb(var(--gray-10));

			&:nth-child(even){
				background: rgb(var(--gray-9));
			}

			&-code{
				display: flex;
				line-height: 2;
				color: var(--color-white);

				&:hover{
					cursor: pointer;
					background: rgb(var(--gray-8));
					& span{
						background: rgb(var(--gray-7));
					}
				}

				& span{
					background: rgb(var(--gray-8));
					border-right: rgb(var(--gray-7)) solid 2px;
					box-sizing: border-box;
					display: inline-block;
					text-align: right;
					padding: 0 8px;
					width: 60px;
					color: #999
				}
				&>div{
					flex:1;
					padding: 0 12px;
					word-break: break-all;
					-webkit-line-clamp: 2;
					-webkit-box-orient: vertical;
					display: -webkit-box;
					overflow: hidden;
				}
			}
		}
		&-box {
			height: 100%;
			width: 100%;
			overflow-y: auto;
		}
		&-modal{
			word-break: break-all;
			white-space: pre-wrap;
		}
		.back-top-btn{
			position: absolute;
			right: 40px;
			bottom: 40px;
		}
	}

</style>
