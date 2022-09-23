<template>
	<a-layout class="layout-demo">
		<a-layout-sider hide-trigger
										:collapsed-width="0"
										collapsible
										:collapsed="collapsed">
			<opts/>
		</a-layout-sider>
		<a-layout>
			<a-layout-header>
				<Header>
					<a-button type="text" class="a-menu-btn" @click="onCollapse">
						<IconCaretRight v-if="collapsed"/>
						<IconCaretLeft v-else/>
					</a-button>
				</Header>
			</a-layout-header>
			<a-layout-content>
				<Code></Code>
			</a-layout-content>
		</a-layout>

	</a-layout>
</template>

<script setup>
	import ws from './utils/ws.js';
	import Opts from "./components/opts.vue";
	import Code from './components/code.vue';
	import Header from './components/header.vue';

	import {
		IconCaretRight,
		IconCaretLeft,
	} from '@arco-design/web-vue/es/icon';
	import {ref} from 'vue'

	const collapsed = ref(false);
	const onCollapse = () => {
		collapsed.value = !collapsed.value;
	};
	ws.open()

</script>


<style lang="css">
	.layout-demo {
		height: 100vh;
	}

	.el-menu-btn {
		position: absolute;
		right: -45px;
		z-index: 2;
	}
</style>
