<script setup>
import {onMounted, ref, reactive} from 'vue'
import dayjs from 'dayjs';
import { Notification } from '@arco-design/web-vue'
import ws from '../utils/ws'
import '@arco-design/web-vue/es/scrollbar/style/index.less'
import '@arco-design/web-vue/es/notification/style/index.less'

const form = reactive({
  application: '',
  system: '',
  since: '',
  until: '',
  lines: 10000,
  grep: '',
})
const dockerList = ref([]);
const systemList = ref([]);
const loading = ref(false);


onMounted(async ()=>{
  const resp = await window.fetch('/readlog/list');
  const ret = await resp.json();
	const [dcoker, sysem] = ret;
	dockerList.value = dcoker.list;
	systemList.value = sysem.list;
})

const onChangeDate = ([start, end])=>{
  form.since = start;
  form.until = end;
}

const changeSystem = (e)=>{
	if(e){
    form.application = ''
    form.since= ''
    form.until= ''
  }
	handleSubmit({values:form});
}
const changeApplation = (e)=>{
	if(e){
    form.system = ''
    form.since= ''
    form.until= ''
  }
  handleSubmit({values:form});
}
const setType = (type)=>{
  type = ['history', 'realtime'].includes(type)?type: 'realtime';
  form.logType = type;
}

const handleSubmit = ({values, errors})=>{
	if(errors) return Notification.error(errors);
	const {application, system, logType, ...others} = values;
	if(!application && !system) return Notification.error('请先选择应用或者系统');
	//const logType = others.since || others.until ?'history':'realtime';
	ws.emit('channelChange', values); //广播通知 更换搜索条件了
  if(logType === 'history') loading.value = true;
	ws.send({
		...others,
		log_type: logType,
		service_type: application?'docker':'systemd',
		service_name: application||system,
	}).finally(()=>{
    loading.value = false;
  })
}

</script>

<template>
  <a-form class="p-5"  :model="form" @submit="handleSubmit" layout="vertical">
    <a-form-item field="name" label="docker:">
      <a-select @change="changeApplation" v-model="form.application" :trigger-props="{ autoFitPopupMinWidth: true }" placeholder="请选择应用...">
        <a-option v-for="i in dockerList" :key="i.value" :value="i.value">{{i.name}}</a-option>
      </a-select>
    </a-form-item>
    <a-form-item field="post" label="journalctl:">
      <a-select @change="changeSystem" v-model="form.system" placeholder="请选择系统..." >
				<a-option v-for="i in systemList" :key="i.value" :value="i.value">{{i.name}}</a-option>
      </a-select>
    </a-form-item>
    <a-form-item field="post" label="时间:">
      <a-range-picker
          show-time
          :time-picker-props="{ defaultValue: '09:09:06' }"
          format="YYYY-MM-DD HH:mm:ss"
          @change="onChangeDate"
          :default-value="[dayjs().subtract(30, 'minute'), dayjs()]"
          :shortcuts="[{
            label: '最近10分钟',
            value: ()=>[dayjs().subtract(10, 'minute'), dayjs()]
          },{
            label: '最近半小时',
            value: ()=>[dayjs().subtract(30, 'minute'), dayjs()]
          },{
            label: '最近1小时',
            value: ()=>[dayjs().subtract(1, 'hour'), dayjs()]
          },{
            label: '最近1天',
            value: ()=>[dayjs().subtract(1, 'day'), dayjs()]
          },
          {
            label: '最近1周',
            value: ()=>[dayjs().subtract(1, 'week'), dayjs()]
          }
          ]"
      ></a-range-picker>
    </a-form-item>
    <a-form-item field="post" label="行数:">
      <a-input-number v-model="form.lines"  placeholder="请输入" :min="1" :max="10000"/>
    </a-form-item>
    <a-form-item field="post" label="关键词:">
      <a-input v-model="form.grep"  placeholder="请输入" />
    </a-form-item>
    <a-form-item>
      <a-space>
        <a-button :loading="loading" @click="setType('history')" html-type="submit" >历史搜索</a-button>
        <a-button @click="setType('realtime')" html-type="submit" type="primary">实时搜索</a-button>
      </a-space>
    </a-form-item>
  </a-form>
</template>

<style scoped>
	.p-5{
		padding: 10px;
	}
</style>
