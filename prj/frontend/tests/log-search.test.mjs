import test from 'node:test'
import assert from 'node:assert/strict'
import {findLogMatches,nextLogMatch} from '../src/utils/log-search.js'
test('literal search preserves offsets and matches latest occurrence',()=>{
 const text='one [ERROR]\ntwo [error] and [error]'
 const matches=findLogMatches(text,'[error]')
 assert.equal(matches.length,3)
 assert.equal(matches.at(-1).start,text.lastIndexOf('[error]'))
 assert.equal(text.slice(matches[0].start,matches[0].end),'[ERROR]')
 assert.deepEqual(findLogMatches(text,''),[])
 assert.deepEqual(findLogMatches(text,'missing'),[])
})
test('previous and next wrap and empty results stay unselected',()=>{
 assert.equal(nextLogMatch(-1,3,1),2)
 assert.equal(nextLogMatch(2,3,1),0)
 assert.equal(nextLogMatch(0,3,-1),2)
 assert.equal(nextLogMatch(0,0,1),-1)
})
