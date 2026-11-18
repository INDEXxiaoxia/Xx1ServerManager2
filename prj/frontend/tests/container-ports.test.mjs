import test from 'node:test'
import assert from 'node:assert/strict'
import {containerPorts,portDescription} from '../src/utils/container-ports.js'
test('groups IPv4/IPv6 mappings without dropping listen addresses',()=>{
 const p=containerPorts('0.0.0.0:8080->80/tcp, [::]:8080->80/tcp, 443/tcp')
 assert.equal(p.length,2);assert.deepEqual(p[0].addresses,['0.0.0.0','[::]'])
 assert.equal(p[0].published,'8080');assert.equal(p[0].target,'80');assert.equal(p[1].published,undefined)
 assert.match(portDescription(p[1]),/未发布/)
})
test('preserves ranges, UDP, IPv6 and unknown formats',()=>{
 const p=containerPorts(':::5000-5005->6000-6005/udp, unknown')
 assert.equal(p[0].protocol,'UDP');assert.equal(p[0].addresses[0],'::');assert.equal(p[1].raw,'unknown')
 assert.deepEqual(containerPorts(''),[])
})
