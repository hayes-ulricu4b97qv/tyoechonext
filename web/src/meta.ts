export function setDocumentMeta(title: string, description = '', keywords = '') {
  document.title = title
  setMeta('description', description)
  setMeta('keywords', keywords)
}

function setMeta(name: string, content: string) {
  let element = document.head.querySelector(`meta[name="${name}"]`)
  if (!(element instanceof HTMLMetaElement)) {
    element = document.createElement('meta')
    element.setAttribute('name', name)
    document.head.appendChild(element)
  }
  element.setAttribute('content', content)
}
