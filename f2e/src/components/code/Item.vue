<template>
	<span>{{source.id}}:</span>
	<div  @click="onHandle">{{source.txt}}</div>
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