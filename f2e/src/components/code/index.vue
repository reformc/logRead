<template>
  <div class="code">
    <div v-if="lines.length>0" class="code-title">
      共: {{ lines.length }} 行
    </div>
    <a-space class="code-opera">
      <a-button @click="exportRaw" type="primary" shape="circle">
        <icon-download/>
      </a-button>
    </a-space>
    <VirtualList
        ref="target"
        class="code-box"
        wrapClass="code-wrapper"
        :data-key="'id'"
        :keeps="150"
        :extra-props="{

					onSelect: onSelect
				}"
        @scroll="onScroll"
        :data-sources="lines"
        :data-component="Item"
    >

    </VirtualList>
    <a-modal width="85%" :footer="null" title="日志信息" v-model:visible="codeVisible" @ok="onCodeHide"
             @cancel="(e)=>onCodeHide">
      <div class="code-modal" v-html="codeTxt"></div>
    </a-modal>
    <a-badge v-if="visible" :count="count" class="back-top-btn">
      <a-button @click="scrollBottom">
        <IconDown/>
      </a-button>
    </a-badge>

  </div>
</template>

<script setup>
import {ref, nextTick, onMounted} from 'vue'
import dayjs from "dayjs";
import VirtualList from 'vue3-virtual-scroll-list'
import ws from '../../utils/ws.js'
import {IconDown, IconSearch, IconDownload} from '@arco-design/web-vue/es/icon'
import format from '../../utils/format';
import Item from './Item.vue';
import debounce from "../../utils/debounce.js";
import {Notification} from "@arco-design/web-vue";

const target = ref(null);
const visible = ref(false);
const count = ref(0);
const lines = ref([]);
const app = ref('');
const scrolling = ref(false);
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
      line.push({id, txt});
    });
    if (line.length > lineNumber) lines.value = line.slice(line.length - lineNumber);
    count.value += data.length;

    nextTick(() => {
      if (!scrolling.value || !visible.value) {
        scrollBottom()
      }
    })
  })
  ws.on('channelChange', (data) => {
    data.application && (app.value = data.application)
    if (data.lines > 0) lineNumber.value = data.lines;
    id = 0;
    lines.value = [];
  })
})

const exportRaw = () => {
  const data = lines.value.map((e) => e.txt).join('');
  const blob = new Blob([data]);
  const URL = window.URL || window.webkitURL || window;
  const link = document.createElementNS('http://www.w3.org/1999/xhtml', 'a');
  link.href = URL.createObjectURL(blob);
  link.download = `${app.value}_${dayjs().format('HHmmss')}_log`
  link.click();
}

const scrollEnd = debounce(() => {
  scrolling.value = false;
}, 300)

const onScroll = (evt) => {
  if (!scrolling.value) scrolling.value = true;
  scrollEnd();
  const el = evt.target;
  const {scrollTop, offsetHeight, scrollHeight} = el;

  if (scrollTop + offsetHeight + 300 >= scrollHeight) {
    visible.value = false;
  } else {
    visible.value = true;
  }
}

const onSelect = (e) => {
  codeVisible.value = true;
  const txt = e.txt;
  codeTxt.value = format(txt);
}
const onCodeHide = () => {
  codeVisible.value = false;
}

</script>

<style lang="less">
.code {
  position: relative;
  height: calc(100vh - 32px);
  width: 100%;

  &-opera {
    position: absolute;
    top: 10px;
    right: 40px;

    & .arco-btn {
      box-shadow: 0 3px 10px var(--color-bg-1);
    }
  }

  &-title {
    position: absolute;
    z-index: 100;
    padding: 6px 10px;
    background: var(--color-bg-1);
    color: var(--color-text-1);
    box-shadow: 0 3px 10px var(--color-bg-2);
  }

  &-box {
    height: 100%;
    width: 100%;
    overflow-x: auto;
    //overflow-x: auto;
  }

  &-wrapper {
    display: block;
  }

  &-line {
    box-sizing: border-box;
    display: flex;
    display: flex;
    line-height: 2.5;
    white-space: pre;
    color: var(--color-text-1);

    &:hover {
      cursor: pointer;
      background: var(--color-bg-2);

      & span {
        background: var(--color-bg-2);
      }
    }

    & span {
      background: var(--color-bg-3);
      //border-right: var(--color-border) solid 2px;
      box-sizing: border-box;
      display: inline-block;
      text-align: right;
      padding: 0 8px;
      flex: 0 0 60px;
      color: var(--color-text-3);
    }

    & > div {
      flex: 1;
      padding: 0 12px;
      //word-break: break-all;
      //-webkit-line-clamp: 2;
      //-webkit-box-orient: vertical;
      //display: -webkit-box;
      //overflow: hidden;
    }
  }

  &-modal {
    word-break: break-all;
    white-space: pre-wrap;
  }

  .back-top-btn {
    position: absolute;
    right: 40px;
    bottom: 40px;
  }
}

</style>
