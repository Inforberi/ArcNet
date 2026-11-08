import {test} from 'node:test';
import assert from 'node:assert/strict';
import {canDismiss} from '../src/shared/dialog-dismiss.ts';

test('busy dialogs cannot be dismissed or ask to discard changes',()=>{
 assert.equal(canDismiss(true,true,()=>{assert.fail('must not ask while saving')}),false);
 assert.equal(canDismiss(false,true),false);
});
test('closing respects unsaved changes without prompting on clean forms',()=>{
 assert.equal(canDismiss(false,false,()=>{assert.fail('clean form')}),true);
 assert.equal(canDismiss(true,false,()=>false),false);
 assert.equal(canDismiss(true,false,()=>true),true);
});
