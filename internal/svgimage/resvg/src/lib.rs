//! resvg for folio, as a WebAssembly module without imports.
//!
//! The host writes into memory it got from `alloc`, adds fonts with
//! `add_font`, and calls `render`, which leaves premultiplied RGBA pixels,
//! or an error message, in the result buffer (`result_ptr`, `result_len`).

use std::sync::Arc;

use resvg::{tiny_skia, usvg};

static mut FONTS: Option<Arc<usvg::fontdb::Database>> = None;
static mut RESULT: Vec<u8> = Vec::new();

#[allow(static_mut_refs)] // the module is single-threaded
fn fonts() -> &'static mut Arc<usvg::fontdb::Database> {
    unsafe { FONTS.get_or_insert_with(Default::default) }
}

/// Reserves len bytes for the host to write into.
#[no_mangle]
pub extern "C" fn alloc(len: usize) -> *mut u8 {
    let mut buf = Vec::<u8>::with_capacity(len);
    let ptr = buf.as_mut_ptr();
    std::mem::forget(buf);
    ptr
}

/// Releases memory from `alloc`.
#[no_mangle]
pub unsafe extern "C" fn dealloc(ptr: *mut u8, len: usize) {
    drop(Vec::from_raw_parts(ptr, 0, len));
}

/// Adds a TrueType or OpenType font, taking over the memory it is in. The
/// first font added stands in for every generic family, and so for any
/// family the fonts do not have.
#[no_mangle]
pub unsafe extern "C" fn add_font(ptr: *mut u8, len: usize) {
    let data = Vec::from_raw_parts(ptr, len, len);
    let db = Arc::make_mut(fonts());
    let first = db.is_empty();
    db.load_font_data(data);
    if first {
        let name = db.faces().next().and_then(|f| f.families.first()).map(|f| f.0.clone());
        if let Some(name) = name {
            db.set_serif_family(name.clone());
            db.set_sans_serif_family(name.clone());
            db.set_monospace_family(name.clone());
            db.set_cursive_family(name.clone());
            db.set_fantasy_family(name);
        }
    }
}

/// Draws the SVG at ptr (len bytes) into a width×height image, scaling its
/// viewBox to fit. It returns 0 with the pixels in the result buffer, or 1
/// with an error message there.
#[no_mangle]
pub unsafe extern "C" fn render(ptr: *const u8, len: usize, width: u32, height: u32) -> u32 {
    let svg = std::slice::from_raw_parts(ptr, len);
    let (out, status) = match draw(svg, width, height) {
        Ok(pixels) => (pixels, 0),
        Err(msg) => (msg.into_bytes(), 1),
    };
    #[allow(static_mut_refs)]
    {
        RESULT = out;
    }
    status
}

fn draw(svg: &[u8], width: u32, height: u32) -> Result<Vec<u8>, String> {
    let mut opt = usvg::Options::default();
    opt.fontdb = fonts().clone();
    // An image may come from a data URL only: the module reads no files.
    opt.image_href_resolver.resolve_string = Box::new(|_, _| None);
    let tree = usvg::Tree::from_data(svg, &opt).map_err(|e| e.to_string())?;
    let mut pixmap = tiny_skia::Pixmap::new(width, height).ok_or("empty image")?;
    let size = tree.size();
    let t = tiny_skia::Transform::from_scale(width as f32 / size.width(), height as f32 / size.height());
    resvg::render(&tree, t, &mut pixmap.as_mut());
    Ok(pixmap.take())
}

#[no_mangle]
pub extern "C" fn result_ptr() -> *const u8 {
    #[allow(static_mut_refs)]
    unsafe {
        RESULT.as_ptr()
    }
}

#[no_mangle]
pub extern "C" fn result_len() -> usize {
    #[allow(static_mut_refs)]
    unsafe {
        RESULT.len()
    }
}
