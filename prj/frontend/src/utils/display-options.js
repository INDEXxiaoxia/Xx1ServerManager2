import {ref, watch} from 'vue'
export const showImageHash = ref(localStorage.getItem('xx1.showImageHash') === 'true')
watch(showImageHash, value => localStorage.setItem('xx1.showImageHash', String(value)))
