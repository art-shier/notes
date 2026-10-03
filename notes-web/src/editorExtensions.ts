import StarterKit from '@tiptap/starter-kit';
import Image from '@tiptap/extension-image';
export const PrivateImage=Image.extend({addAttributes(){return {...this.parent?.(),attachment_id:{default:null}}}});
export const editorExtensions=()=>[StarterKit,PrivateImage.configure({allowBase64:false})];
