<template>
	<div @click="onHandle" class="code-line-code">
		<span>{{source.id}}:</span>
		<div>{{source.txt}}</div>
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