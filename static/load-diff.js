function load(e) {
    e.preventDefault();

    // add .selected class to clicked element
    var selected = document.querySelector('.selected');
    if (selected) {
        selected.classList.remove('selected');
    }
    e.target.classList.add('selected');

    window.parent.document.dispatchEvent(new CustomEvent('loadDiff', {
        detail: {
            tag1: e.target.dataset.tag1,
            tag2: e.target.dataset.tag2,
            file: e.target.dataset.name,
            oldFile: e.target.dataset.oldname
        }
    }));
}
