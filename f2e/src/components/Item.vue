<template>
	<div @click="onHandle" class="code-line-code">
		{{source.txt}}
	</div>
</template>

<script setup>
	const emit = defineEmits(['select']);
	const props = defineProps({
		source: {
			type: Object,
			default: ()=>({
				id: '',
				txt: ''
			}),
		},
	});

	const onHandle = ()=>{
		emit('select', props.source)
	}
</script>

<style scoped>
.line{
	flex:1;
	/*padding: 6px 20px;*/
	word-break: break-all;
	-webkit-line-clamp: 2;
	-webkit-box-orient: vertical;
	display: -webkit-box;
	overflow: hidden;
	/*white-space: nowrap;*/
	/*text-overflow: ellipsis;*/
}
</style>